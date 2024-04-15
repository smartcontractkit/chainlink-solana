package monitoring

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
)

const (
	txresultsType = "txresults"
)

func NewTxResultsSourceFactory(
	client ChainReader,
	log commonMonitoring.Logger,
) commonMonitoring.SourceFactory {
	return &txResultsSourceFactory{
		client,
		log,
	}
}

type txResultsSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (s *txResultsSourceFactory) NewSource(
	_ commonMonitoring.ChainConfig,
	feedConfig commonMonitoring.FeedConfig,
) (commonMonitoring.Source, error) {
	solanaFeedConfig, ok := feedConfig.(config.SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type config.SolanaFeedConfig not %T", feedConfig)
	}
	return &txResultsSource{
		s.client,
		s.log,
		solanaFeedConfig,
		solana.Signature{},
		sync.Mutex{},
	}, nil
}

func (s *txResultsSourceFactory) GetType() string {
	return txresultsType
}

type txResultsSource struct {
	client     ChainReader
	log        commonMonitoring.Logger
	feedConfig config.SolanaFeedConfig

	latestSig   solana.Signature
	latestSigMu sync.Mutex
}

// Fetch is the externally called method that returns the specific TxResults output
func (t *txResultsSource) Fetch(ctx context.Context) (interface{}, error) {
	out, _, err := t.fetch(ctx)
	return out, err
}

// fetch is the internal method that returns data from the GetSignaturesForAddress RPC call
func (t *txResultsSource) fetch(ctx context.Context) (commonMonitoring.TxResults, []*rpc.TransactionSignature, error) {
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
		return commonMonitoring.TxResults{}, nil, fmt.Errorf("failed to fetch transactions for state account: %w", err)
	}
	if len(txSigs) == 0 {
		return commonMonitoring.TxResults{NumSucceeded: 0, NumFailed: 0}, nil, nil
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
	return commonMonitoring.TxResults{NumSucceeded: numSucceeded, NumFailed: numFailed}, txSigs, nil
}
