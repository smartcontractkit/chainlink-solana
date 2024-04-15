package monitoring

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
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

	// TODO: build map for looking up nodes

	return &txDetailsSource{
		source: &txResultsSource{
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
	nodes map[solana.PublicKey]string // track pubkey to operator name

	source *txResultsSource // reuse underlying logic for getting signatures
}

func (s *txDetailsSource) Fetch(ctx context.Context) (interface{}, error) {
	_, sigs, err := s.source.fetch(ctx)
	if err != nil {
		return types.TxDetails{}, err
	}
	if len(sigs) == 0 {
		return types.TxDetails{}, nil
	}

	details := types.TxDetails{}
	for _, sig := range sigs {
		if sig == nil {
			continue // skip for nil signatures
		}

		// TODO: async?
		tx, err := s.source.client.GetTransaction(ctx, sig.Signature, &rpc.GetTransactionOpts{Commitment: "confirmed"})
		if err != nil {
			return types.TxDetails{}, err
		}
		if tx == nil {
			return types.TxDetails{}, fmt.Errorf("GetTransaction returned nil")
		}

		// parse transaction + filter based on known senders
		res, err := types.ParseTxResult(tx, s.nodes, s.source.feedConfig.ContractAddress)
		if err != nil {
			// skip invalid transaction
			s.source.log.Debugw("tx was not valid for tracking", "error", err, "signature", sig)
			continue
		}

		// append to TxDetails
		details.Count += 1
	}

	return details, nil
}
