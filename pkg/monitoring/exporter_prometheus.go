package monitoring

import (
	"context"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
)

func NewPrometheusExporter(
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
	log logger.Logger,
	metrics Metrics,
) Exporter {
	metrics.SetFeedContractMetadata(
		solanaConfig.ChainID,
		feedConfig.GetContractAddress(),
		feedConfig.GetContractAddress(),
		feedConfig.GetContractStatus(),
		feedConfig.GetContractType(),
		feedConfig.GetName(),
		feedConfig.GetPath(),
		solanaConfig.NetworkID,
		solanaConfig.NetworkName,
		feedConfig.GetSymbol(),
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
	feedConfig   FeedConfig

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
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			p.solanaConfig.ChainID,
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)
		p.metrics.IncOffchainAggregatorAnswersTotal(
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			p.solanaConfig.ChainID,
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)

		isLateAnswer := time.Since(typed.LatestTimestamp).Seconds() > float64(p.feedConfig.GetHeartbeatSec())
		p.metrics.SetOffchainAggregatorAnswerStalled(
			isLateAnswer,
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			p.solanaConfig.ChainID,
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)
		p.metrics.SetOffchainAggregatorSubmissionReceivedValues(
			typed.LatestAnswer,
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			"n/a", // sender
			p.solanaConfig.ChainID,
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)
	default:
		p.log.Errorf("unexpected type %T for export", data)
	}
}
