package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
)

func NewPrometheusExporter(
	solanaConfig config.Solana,
	feedConfig Feed,
	log logger.Logger,
	metrics Metrics,
) Exporter {
	metrics.SetFeedContractMetadata(
		solanaConfig.ChainID,
		feedConfig.ContractAddress.String(),
		feedConfig.StateAccount.String(),
		feedConfig.ContractStatus,
		feedConfig.ContractType,
		feedConfig.FeedName,
		feedConfig.FeedPath,
		solanaConfig.NetworkID,
		solanaConfig.NetworkName,
		feedConfig.Symbol,
	)

	return &prometheusExporter{
		solanaConfig,
		feedConfig,
		log,
		metrics,
		"n/a",
		sync.Mutex{},
	}
}

type prometheusExporter struct {
	solanaConfig config.Solana
	feedConfig   Feed

	log     logger.Logger
	metrics Metrics

	// The transmissions account does not record the latest transmitter node.
	// Instead, the state account stores the latest transmitter node's public key.
	// We store the latest transmitter in memory to be used to associate the latest update.
	latestTransmitter   string
	latestTransmitterMu sync.Mutex
}

func (p *prometheusExporter) Export(ctx context.Context, data interface{}) {
	switch typed := data.(type) {
	case StateEnvelope:
		p.metrics.SetNodeMetadata(
			p.solanaConfig.ChainID,
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
			"n/a",
			typed.State.Config.LatestTransmitter.String(),
		)

		func() {
			p.latestTransmitterMu.Lock()
			defer p.latestTransmitterMu.Unlock()
			p.latestTransmitter = typed.State.Config.LatestTransmitter.String()
		}()
	case TransmissionEnvelope:
		p.metrics.SetHeadTrackerCurrentHead(
			typed.BlockNumber,
			p.solanaConfig.NetworkName,
			p.solanaConfig.ChainID,
			p.solanaConfig.NetworkID,
		)
		p.metrics.SetOffchainAggregatorAnswers(
			typed.Answer.Data,
			p.feedConfig.ContractAddress.String(),
			p.feedConfig.StateAccount.String(),
			p.solanaConfig.ChainID,
			p.feedConfig.ContractStatus,
			p.feedConfig.ContractType,
			p.feedConfig.FeedName,
			p.feedConfig.FeedPath,
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)
		p.metrics.IncOffchainAggregatorAnswersTotal(
			p.feedConfig.ContractAddress.String(),
			p.feedConfig.StateAccount.String(),
			p.solanaConfig.ChainID,
			p.feedConfig.ContractStatus,
			p.feedConfig.ContractType,
			p.feedConfig.FeedName,
			p.feedConfig.FeedPath,
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)

		isLateAnswer := time.Since(time.Unix(int64(typed.Answer.Timestamp), 0)).Seconds() > float64(p.feedConfig.HeartbeatSec)
		p.metrics.SetOffchainAggregatorAnswerStalled(
			isLateAnswer,
			p.feedConfig.ContractAddress.String(),
			p.feedConfig.StateAccount.String(),
			p.solanaConfig.ChainID,
			p.feedConfig.ContractStatus,
			p.feedConfig.ContractType,
			p.feedConfig.FeedName,
			p.feedConfig.FeedPath,
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)

		func() {
			p.latestTransmitterMu.Lock()
			defer p.latestTransmitterMu.Unlock()
			p.metrics.SetOffchainAggregatorSubmissionReceivedValues(
				typed.Answer.Data,
				p.feedConfig.ContractAddress.String(),
				p.feedConfig.StateAccount.String(),
				p.latestTransmitter,
				p.solanaConfig.ChainID,
				p.feedConfig.ContractStatus,
				p.feedConfig.ContractType,
				p.feedConfig.FeedName,
				p.feedConfig.FeedPath,
				p.solanaConfig.NetworkID,
				p.solanaConfig.NetworkName,
			)
		}()
	default:
		p.log.Errorf("unexpected type %T for export", data)
	}
}
