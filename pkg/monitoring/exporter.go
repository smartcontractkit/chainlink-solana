package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
)

type Exporter interface {
	Export(ctx context.Context, data interface{})
}

func NewPrometheusExporter(
	solanaConfig config.Solana,
	feedConfig config.Feed,
	log logger.Logger,
	metrics Metrics,
) Exporter {
	metrics.SetFeedContractMetadata(
		solanaConfig.ChainID, feedConfig.ContractAddress.String(),
		feedConfig.ContractStatus, feedConfig.ContractType, feedConfig.FeedName,
		feedConfig.FeedPath, solanaConfig.NetworkID, solanaConfig.NetworkName,
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
	feedConfig   config.Feed

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
			p.solanaConfig.ChainID, p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName, "n/a",
			typed.State.Config.LatestTransmitter.String(),
		)

		func() {
			p.latestTransmitterMu.Lock()
			defer p.latestTransmitterMu.Unlock()
			p.latestTransmitter = typed.State.Config.LatestTransmitter.String()
		}()
	case TransmissionEnvelope:
		p.metrics.SetHeadTrackerCurrentHead(
			typed.BlockNumber, p.solanaConfig.NetworkName,
			p.solanaConfig.ChainID, p.solanaConfig.NetworkID,
		)
		p.metrics.SetOffchainAggregatorAnswers(
			typed.Answer.Data, p.feedConfig.ContractAddress.String(),
			p.solanaConfig.ChainID, p.feedConfig.ContractStatus,
			p.feedConfig.ContractType, p.feedConfig.FeedName,
			p.feedConfig.FeedPath, p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)
		p.metrics.IncOffchainAggregatorAnswersTotal(
			p.feedConfig.ContractAddress.String(),
			p.solanaConfig.ChainID, p.feedConfig.ContractStatus,
			p.feedConfig.ContractType, p.feedConfig.FeedName,
			p.feedConfig.FeedPath, p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)

		isLateAnswer := time.Since(time.Unix(int64(typed.Answer.Timestamp), 0)).Seconds() > float64(p.feedConfig.HeartbeatSec)
		p.metrics.SetOffchainAggregatorAnswerStalled(
			isLateAnswer, p.feedConfig.ContractAddress.String(),
			p.solanaConfig.ChainID, p.feedConfig.ContractStatus,
			p.feedConfig.ContractType, p.feedConfig.FeedName,
			p.feedConfig.FeedPath, p.solanaConfig.NetworkID,
			p.solanaConfig.NetworkName,
		)

		func() {
			p.latestTransmitterMu.Lock()
			defer p.latestTransmitterMu.Unlock()
			p.metrics.SetOffchainAggregatorSubmissionReceivedValues(
				typed.Answer.Data, p.feedConfig.ContractAddress.String(),
				p.latestTransmitter, p.solanaConfig.ChainID,
				p.feedConfig.ContractStatus, p.feedConfig.ContractType,
				p.feedConfig.FeedName, p.feedConfig.FeedPath,
				p.solanaConfig.NetworkID, p.solanaConfig.NetworkName,
			)
		}()
	default:
		p.log.Errorf("unexpected type %T for export", data)
	}
}

func NewKafkaExporter(
	solanaConfig config.Solana,
	feedConfig config.Feed,

	log logger.Logger,
	producer Producer,

	configSetSchema Schema,
	configSetSimplifiedSchema Schema,
	transmissionSchema Schema,

	configSetTopic string,
	configSetSimplifiedTopic string,
	transmissionTopic string,
) Exporter {
	return &kafkaExporter{
		solanaConfig,
		feedConfig,

		log,
		producer,

		configSetSchema,
		configSetSimplifiedSchema,
		transmissionSchema,

		configSetTopic,
		configSetSimplifiedTopic,
		transmissionTopic,
	}
}

type kafkaExporter struct {
	solanaConfig config.Solana
	feedConfig   config.Feed

	log      logger.Logger
	producer Producer

	configSetSchema           Schema
	configSetSimplifiedSchema Schema
	transmissionSchema        Schema

	configSetTopic           string
	configSetSimplifiedTopic string
	transmissionTopic        string
}

func (k *kafkaExporter) Export(ctx context.Context, data interface{}) {
	key := k.feedConfig.StateAccount.Bytes()

	switch typed := data.(type) {
	case StateEnvelope:
		func() {
			configSetMapping, err := MakeConfigSetMapping(typed, k.solanaConfig, k.feedConfig)
			if err != nil {
				k.log.Errorw("failed to map config_set", "error", err)
				return
			}
			configSetEncoded, err := k.configSetSchema.Encode(configSetMapping)
			if err != nil {
				k.log.Errorw("failed to encode config_set to Avro", "payload", configSetMapping, "error", err)
				return
			}
			if err := k.producer.Produce(key, configSetEncoded, k.configSetTopic); err != nil {
				k.log.Errorw("failed to publish config_set", "payload", configSetMapping, "error", err)
				return
			}
		}()

		func() {
			configSetSimplifiedMapping, err := MakeConfigSetSimplifiedMapping(typed, k.feedConfig)
			if err != nil {
				k.log.Errorw("failed to map config_set_simplified", "error", err)
				return
			}
			configSetSimplifiedEncoded, err := k.configSetSimplifiedSchema.Encode(configSetSimplifiedMapping)
			if err != nil {
				k.log.Errorw("failed to encode config_set_simplified to Avro", "payload", configSetSimplifiedMapping, "error", err)
				return
			}
			if err := k.producer.Produce(key, configSetSimplifiedEncoded, k.configSetSimplifiedTopic); err != nil {
				k.log.Errorw("failed to publish config_set_simplified", "payload", configSetSimplifiedMapping, "error", err)
				return
			}
		}()
	case TransmissionEnvelope:
		transmissionMapping, err := MakeTransmissionMapping(typed, k.solanaConfig, k.feedConfig)
		if err != nil {
			k.log.Errorw("failed to map transmission", "error", err)
			return
		}
		transmissionEncoded, err := k.transmissionSchema.Encode(transmissionMapping)
		if err != nil {
			k.log.Errorw("failed to encode transmission to Avro", "payload", transmissionMapping, "error", err)
			return
		}
		if err := k.producer.Produce(key, transmissionEncoded, k.transmissionTopic); err != nil {
			k.log.Errorw("failed to publish transmission", "payload", transmissionMapping, "error", err)
			return
		}
	default:
		k.log.Errorf("unknown type %T to export", data)
	}
}
