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

	// build map for looking up node pubkey -> operator name
	nodes := map[solana.PublicKey]string{}
	solanaNodeConfigs, err := config.MakeSolanaNodeConfigs(cfg.Nodes)
	if err != nil {
		return nil, fmt.Errorf("MakeSolanaNodeConfigs: %w", err)
	}
	for _, c := range solanaNodeConfigs {
		key, err := c.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("Could not parse public key (%s: %s): %w", c.GetName(), c.GetAccount(), err)
		}
		nodes[key] = c.GetName()
	}

	return &txDetailsSource{
		nodes: nodes,
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

		// signatures are ordered with the latest first
		if res.Err == nil && details.ObsLatest == 0 {
			details.ObsLatest = res.ObservationCount // only supports single feed result
		}

		if res.Err != nil {
			details.ObsFailed = append(details.ObsFailed, res.ObservationCount)
		} else {
			details.ObsSuccess = append(details.ObsSuccess, res.ObservationCount)
		}
		details.ObsAll = append(details.ObsAll, res.ObservationCount)

	}

	return details, nil
}
