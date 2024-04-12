package monitoring

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewTxDetailsSourceFactory(client ChainReader, log commonMonitoring.Logger) commonMonitoring.SourceFactory {
	return &txDetailsSourceFactory{client, log}
}

type txDetailsSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (f *txDetailsSourceFactory) NewSource(cfg commonMonitoring.Params) (commonMonitoring.Source, error) {
	solanaFeedConfig, ok := cfg.FeedConfig.(config.SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type config.SolanaFeedConfig not %T", cfg.FeedConfig)
	}

	return &txDetailsSource{
		client: f.client,
		sigSource: &txResultsSource{
			client:     f.client,
			log:        f.log,
			feedConfig: solanaFeedConfig,
		},
	}, nil
}

func (f *txDetailsSourceFactory) GetType() string {
	return types.TxDetailsType
}

type txDetailsSource struct {
	client    ChainReader
	sigSource *txResultsSource // reuse underlying logic for getting signatures
}

func (s *txDetailsSource) Fetch(ctx context.Context) (interface{}, error) {
	_, sigs, err := s.sigSource.fetch(ctx)
	if err != nil {
		return types.TxDetails{}, err
	}
	if len(sigs) == 0 {
		return types.TxDetails{}, nil
	}

	for _, sig := range sigs {
		if sig == nil {
			continue // skip for nil signatures
		}

		// TODO: async?
		tx, err := s.client.GetTransaction(ctx, sig.Signature, &rpc.GetTransactionOpts{})
		if err != nil {
			return types.TxDetails{}, err
		}
		if tx == nil {
			return types.TxDetails{}, fmt.Errorf("GetTransaction returned nil")
		}

		// TODO: parse transaction

		// TODO: filter signatures/transactions based on known operator/sender

		// TODO: parse observations from remaining transactions

		// TODO: add to proper list for averaging
	}

	return types.TxDetails{}, nil
}
