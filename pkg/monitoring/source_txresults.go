package monitoring

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewTxResultsSourceFactory(
	client *rpc.Client,
	log relayMonitoring.Logger,
) relayMonitoring.SourceFactory {
	return &txResultsSourceFactory{
		client,
		log,
	}
}

type txResultsSourceFactory struct {
	client *rpc.Client
	log    relayMonitoring.Logger
}

func (s *txResultsSourceFactory) NewSource(
	_ relayMonitoring.ChainConfig,
	feedConfig relayMonitoring.FeedConfig,
) (relayMonitoring.Source, error) {
	solanaFeedConfig, ok := feedConfig.(SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type SolanaFeedConfig not %T", feedConfig)
	}
	return &txResultsSource{
		s.client,
		s.log,
		solanaFeedConfig,
		solana.Signature{},
		sync.Mutex{},
	}, nil
}

type txResultsSource struct {
	client     *rpc.Client
	log        relayMonitoring.Logger
	feedConfig SolanaFeedConfig

	latestSig   solana.Signature
	latestSigMu sync.Mutex
}

func (t *txResultsSource) Fetch(ctx context.Context) (interface{}, error) {
	txSigsPageSize := 100
	txSigs, err := t.client.GetSignaturesForAddressWithOpts(
		ctx,
		t.feedConfig.StateAccount,
		&rpc.GetSignaturesForAddressOpts{
			Commitment: rpc.CommitmentConfirmed,
			Until:      t.latestSig,
			Limit:      &txSigsPageSize,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transactions for state account: %w", err)
	}
	if len(txSigs) == 0 {
		return relayMonitoring.TxResults{NumSucceeded: 0, NumFailed: 0}, nil
	}
	var numSucceeded, numFailed uint64 = 0, 0
	for _, txSig := range txSigs {
		if txSig.Err == nil {
			numSucceeded++
		} else {
			numFailed++
		}
	}
	func() {
		t.latestSigMu.Lock()
		defer t.latestSigMu.Unlock()
		t.latestSig = txSigs[0].Signature
	}()
	return relayMonitoring.TxResults{NumSucceeded: numSucceeded, NumFailed: numFailed}, nil
}
