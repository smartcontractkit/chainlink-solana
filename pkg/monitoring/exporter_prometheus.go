package monitoring

import (
	"context"
	"sync"
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
	p := &prometheusExporter{
		chainConfig,
		feedConfig,
		log,
		metrics,
		prometheusLabels{},
		sync.Mutex{},
	}
	p.updateLabels(prometheusLabels{
		networkName:     chainConfig.GetNetworkName(),
		networkID:       chainConfig.GetNetworkID(),
		chainID:         chainConfig.GetChainID(),
		feedName:        feedConfig.GetName(),
		feedPath:        feedConfig.GetPath(),
		symbol:          feedConfig.GetSymbol(),
		contractType:    feedConfig.GetContractType(),
		contractStatus:  feedConfig.GetContractStatus(),
		contractAddress: feedConfig.GetContractAddress(),
		feedID:          feedConfig.GetContractAddress(),
	})
	return p
}

type prometheusExporter struct {
	chainConfig ChainConfig
	feedConfig  FeedConfig

	log     logger.Logger
	metrics Metrics

	labels   prometheusLabels
	labelsMu sync.Mutex
}

type prometheusLabels struct {
	networkName     string
	networkID       string
	chainID         string
	oracleName      string
	sender          string
	feedName        string
	feedPath        string
	symbol          string
	contractType    string
	contractStatus  string
	contractAddress string
	feedID          string
}

func (p *prometheusExporter) updateLabels(newLabels prometheusLabels) {
	p.labelsMu.Lock()
	defer p.labelsMu.Unlock()
	if newLabels.networkName != "" {
		p.labels.networkName = newLabels.networkName
	}
	if newLabels.networkID != "" {
		p.labels.networkID = newLabels.networkID
	}
	if newLabels.chainID != "" {
		p.labels.chainID = newLabels.chainID
	}
	if newLabels.oracleName != "" {
		p.labels.oracleName = newLabels.oracleName
	}
	if newLabels.sender != "" {
		p.labels.sender = newLabels.sender
	}
	if newLabels.feedName != "" {
		p.labels.feedName = newLabels.feedName
	}
	if newLabels.feedPath != "" {
		p.labels.feedPath = newLabels.feedPath
	}
	if newLabels.symbol != "" {
		p.labels.symbol = newLabels.symbol
	}
	if newLabels.contractType != "" {
		p.labels.contractType = newLabels.contractType
	}
	if newLabels.contractStatus != "" {
		p.labels.contractStatus = newLabels.contractStatus
	}
	if newLabels.contractAddress != "" {
		p.labels.contractAddress = newLabels.contractAddress
	}
	if newLabels.feedID != "" {
		p.labels.feedID = newLabels.feedID
	}
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
		p.updateLabels(prometheusLabels{
			oracleName: "n/a",
			sender:     "n/a",
		})
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

func (p *prometheusExporter) Cleanup() {
	p.labelsMu.Lock()
	defer p.labelsMu.Unlock()
	p.metrics.Cleanup(
		p.labels.networkName,
		p.labels.networkID,
		p.labels.chainID,
		p.labels.oracleName,
		p.labels.sender,
		p.labels.feedName,
		p.labels.feedPath,
		p.labels.symbol,
		p.labels.contractType,
		p.labels.contractStatus,
		p.labels.contractAddress,
		p.labels.feedID,
	)
}
