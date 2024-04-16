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

func (f *txDetailsSourceFactory) NewSource(_ commonMonitoring.ChainConfig, feedConfig commonMonitoring.FeedConfig) (commonMonitoring.Source, error) {
	solanaFeedConfig, ok := feedConfig.(config.SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type config.SolanaFeedConfig not %T", feedConfig)
	}

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
	source *txResultsSource // reuse underlying logic for getting signatures
}

func (s *txDetailsSource) Fetch(ctx context.Context) (interface{}, error) {
	_, sigs, err := s.source.fetch(ctx)
	if err != nil {
		return nil, err
	}

	details := []types.TxDetails{}
	for _, sig := range sigs {
		if sig == nil {
			continue // skip for nil signatures
		}

		// check only successful txs: indicates the fastest submissions of a report
		if sig.Err != nil {
			continue
		}

		// TODO: worker pool - how many GetTransaction requests in a row?
		tx, err := s.source.client.GetTransaction(ctx, sig.Signature, &rpc.GetTransactionOpts{Commitment: "confirmed"})
		if err != nil {
			return nil, err
		}
		if tx == nil {
			// skip nil transaction (not found)
			s.source.log.Debugw("GetTransaction returned nil", "signature", sig)
			continue
		}

		// parse transaction + filter based on known senders
		res, err := types.ParseTxResult(tx, s.source.feedConfig.ContractAddress)
		if err != nil {
			// skip invalid transaction
			s.source.log.Debugw("tx not valid for tracking", "error", err, "signature", sig)
			continue
		}
		details = append(details, res)
	}

	// only return successful OCR2 transmit transactions
	return details, nil
}
