package monitoring

import (
	"context"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
)

func NewPrometheusExporter(
	chainConfig ChainConfig,
	feedConfig FeedConfig,
	log logger.Logger,
	metrics Metrics,
) Exporter {
	metrics.SetFeedContractMetadata(
		chainConfig.GetChainID(),
		feedConfig.GetContractAddress(),
		feedConfig.GetContractAddress(),
		feedConfig.GetContractStatus(),
		feedConfig.GetContractType(),
		feedConfig.GetName(),
		feedConfig.GetPath(),
		chainConfig.GetNetworkID(),
		chainConfig.GetNetworkName(),
		feedConfig.GetSymbol(),
	)

	return &prometheusExporter{
		chainConfig,
		feedConfig,
		log,
		metrics,
	}
}

type prometheusExporter struct {
	chainConfig ChainConfig
	feedConfig  FeedConfig

	log     logger.Logger
	metrics Metrics
}

func (p *prometheusExporter) Export(ctx context.Context, data interface{}) {
	switch typed := data.(type) {
	case ConfigEnvelope:
		p.metrics.SetNodeMetadata(
			p.chainConfig.GetChainID(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
			"n/a", // oracleName
			"n/a", // sender
		)
	case TransmissionEnvelope:
		p.metrics.SetHeadTrackerCurrentHead(
			0, // block number
			p.chainConfig.GetNetworkName(),
			p.chainConfig.GetChainID(),
			p.chainConfig.GetNetworkID(),
		)
		p.metrics.SetOffchainAggregatorAnswers(
			typed.LatestAnswer,
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			p.chainConfig.GetChainID(),
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
		)
		p.metrics.IncOffchainAggregatorAnswersTotal(
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			p.chainConfig.GetChainID(),
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
		)

		isLateAnswer := time.Since(typed.LatestTimestamp).Seconds() > float64(p.feedConfig.GetHeartbeatSec())
		p.metrics.SetOffchainAggregatorAnswerStalled(
			isLateAnswer,
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			p.chainConfig.GetChainID(),
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
		)
		p.metrics.SetOffchainAggregatorSubmissionReceivedValues(
			typed.LatestAnswer,
			p.feedConfig.GetContractAddress(),
			p.feedConfig.GetContractAddress(),
			"n/a", // sender
			p.chainConfig.GetChainID(),
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
		)
	default:
		p.log.Errorf("unexpected type %T for export", data)
	}
}
