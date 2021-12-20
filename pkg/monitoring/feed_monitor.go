package monitoring

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
)

type FeedMonitor interface {
	Start(ctx context.Context)
}

func NewFeedMonitor(
	log logger.Logger,
	solanaConfig config.Solana,
	feedConfig config.Feed,
	configSetTopic, configSetSimplifiedTopic, transmissionTopic string,
	transmissionPoller, statePoller Poller,
	transmissionSchema, stateSchema, configSetSimplified Schema,
	producer Producer,
	metrics Metrics,
) FeedMonitor {
	return &feedMonitor{
		log,
		solanaConfig,
		feedConfig,
		configSetTopic, configSetSimplifiedTopic, transmissionTopic,
		transmissionPoller, statePoller,
		transmissionSchema, stateSchema, configSetSimplified,
		producer,
		metrics,
	}
}

type feedMonitor struct {
	log                       logger.Logger
	solanaConfig              config.Solana
	feedConfig                config.Feed
	configSetTopic            string
	configSetSimplifiedTopic  string
	transmissionTopic         string
	transmissionPoller        Poller
	statePoller               Poller
	transmissionSchema        Schema
	stateSchema               Schema
	configSetSimplifiedSchema Schema
	producer                  Producer
	metrics                   Metrics
}

// Start should be executed as a goroutine
func (f *feedMonitor) Start(ctx context.Context) {
	f.log.Info("starting feed monitor")
	f.metrics.SetFeedContractMetadata(f.solanaConfig.ChainID, f.feedConfig.ContractAddress.String(),
		f.feedConfig.ContractStatus, f.feedConfig.ContractType, f.feedConfig.FeedName,
		f.feedConfig.FeedPath, f.solanaConfig.NetworkID, f.solanaConfig.NetworkName,
		f.feedConfig.Symbol)

	latestTransmitter := ""
	var latestAnswer *big.Int = nil
	for {
		// Wait for an update.
		var update interface{}
		select {
		case stateRaw := <-f.statePoller.Updates():
			update = stateRaw
		case answerRaw := <-f.transmissionPoller.Updates():
			update = answerRaw
		case <-ctx.Done():
			return
		}
		var err error
		switch typed := update.(type) {
		case StateEnvelope:
			err = f.processConfigSetSimplified(typed)
			if err != nil {
				break
			}

			err = f.processState(typed)
			if err != nil {
				break
			}
			latestTransmitter = typed.State.Config.LatestTransmitter.String()
			f.metrics.SetNodeMetadata(f.solanaConfig.ChainID, f.solanaConfig.NetworkID,
				f.solanaConfig.NetworkName, "n/a", latestTransmitter)
			if latestAnswer != nil {
				f.metrics.SetOffchainAggregatorSubmissionReceivedValues(latestAnswer,
					f.feedConfig.ContractAddress.String(), latestTransmitter, f.solanaConfig.ChainID,
					f.feedConfig.ContractStatus, f.feedConfig.ContractType, f.feedConfig.FeedName,
					f.feedConfig.FeedPath, f.solanaConfig.NetworkID, f.solanaConfig.NetworkName)
			}
		case TransmissionEnvelope:
			err = f.processTransmission(typed)
			latestAnswer = typed.Answer.Data
			f.metrics.SetHeadTrackerCurrentHead(typed.BlockNumber, f.solanaConfig.NetworkName,
				f.solanaConfig.ChainID, f.solanaConfig.NetworkID)
			f.metrics.SetOffchainAggregatorAnswers(latestAnswer, f.feedConfig.ContractAddress.String(),
				f.solanaConfig.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
				f.feedConfig.FeedName, f.feedConfig.FeedPath, f.solanaConfig.NetworkID,
				f.solanaConfig.NetworkName)
			f.metrics.IncOffchainAggregatorAnswersTotal(f.feedConfig.ContractAddress.String(),
				f.solanaConfig.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
				f.feedConfig.FeedName, f.feedConfig.FeedPath, f.solanaConfig.NetworkID,
				f.solanaConfig.NetworkName)
			isLateAnswer := time.Since(time.Unix(int64(typed.Answer.Timestamp), 0)).Seconds() > float64(f.feedConfig.HeartbeatSec)
			f.metrics.SetOffchainAggregatorAnswerStalled(isLateAnswer, f.feedConfig.ContractAddress.String(),
				f.solanaConfig.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
				f.feedConfig.FeedName, f.feedConfig.FeedPath, f.solanaConfig.NetworkID,
				f.solanaConfig.NetworkName)
		default:
			err = fmt.Errorf("unknown update type %T", update)
		}
		if err != nil {
			log.Printf("failed to send message %T: %v", update, err)
			continue
		}
	}
}

func (f *feedMonitor) processState(envelope StateEnvelope) error {
	var mapping map[string]interface{}
	mapping, err := MakeConfigSetMapping(envelope, f.solanaConfig, f.feedConfig)
	if err != nil {
		return fmt.Errorf("failed to map message %v: %w", envelope, err)
	}
	value, err := f.stateSchema.Encode(mapping)
	if err != nil {
		return fmt.Errorf("failed to encode message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value, f.configSetTopic); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	return nil
}

func (f *feedMonitor) processTransmission(envelope TransmissionEnvelope) error {
	var mapping map[string]interface{}
	mapping, err := MakeTransmissionMapping(envelope, f.solanaConfig, f.feedConfig)
	if err != nil {
		return fmt.Errorf("failed to map message %v: %w", envelope, err)
	}
	value, err := f.transmissionSchema.Encode(mapping)
	if err != nil {
		return fmt.Errorf("failed to encode message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value, f.transmissionTopic); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	return nil
}

func (f *feedMonitor) processConfigSetSimplified(envelope StateEnvelope) error {
	var mapping map[string]interface{}
	mapping, err := MakeConfigSetSimplifiedMapping(envelope, f.feedConfig)
	if err != nil {
		return fmt.Errorf("failed to map message %v: %w", envelope, err)
	}
	value, err := f.configSetSimplifiedSchema.Encode(mapping)
	if err != nil {
		return fmt.Errorf("failed to encode message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value, f.configSetSimplifiedTopic); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	return nil
}
