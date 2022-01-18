package monitoring

import (
	"context"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
)

func NewPrometheusExporter(
	solanaConfig SolanaConfig,
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
	}
}

type prometheusExporter struct {
	solanaConfig SolanaConfig
	feedConfig   Feed

	log     logger.Logger
	metrics Metrics
}

func (p *prometheusExporter) Export(ctx context.Context, data interface{}) {
	switch typed := data.(type) {
	case ConfigEnvelope:
		p.metrics.SetNodeMetadata(
			p.solanaConfig.ChainID,
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
			"n/a", // oracleName
			"n/a", // sender
		)
	case TransmissionEnvelope:
		p.metrics.SetHeadTrackerCurrentHead(
			0, // block number
			p.solanaConfig.NetworkName,
			p.solanaConfig.ChainID,
			p.solanaConfig.NetworkID,
		)
		p.metrics.SetOffchainAggregatorAnswers(
			typed.LatestAnswer,
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

		isLateAnswer := time.Since(typed.LatestTimestamp).Seconds() > float64(p.feedConfig.HeartbeatSec)
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
		p.metrics.SetOffchainAggregatorSubmissionReceivedValues(
			typed.LatestAnswer,
			p.feedConfig.ContractAddress.String(),
			p.feedConfig.StateAccount.String(),
			"n/a", // sender
			p.solanaConfig.ChainID,
			p.feedConfig.ContractStatus,
			p.feedConfig.ContractType,
			p.feedConfig.FeedName,
			p.feedConfig.FeedPath,
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)
	default:
		p.log.Errorf("unexpected type %T for export", data)
	}
}
