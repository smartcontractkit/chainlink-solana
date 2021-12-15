package monitoring

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
)

type FeedMonitor interface {
	Start(ctx context.Context)
}

func NewFeedMonitor(
	log logger.Logger,
	config Config,
	feedConfig FeedConfig,
	transmissionPoller, statePoller Poller,
	transmissionSchema, stateSchema, configSetSimplified Schema,
	producer Producer,
	metrics Metrics,
) FeedMonitor {
	return &feedMonitor{
		log,
		config,
		feedConfig,
		transmissionPoller, statePoller,
		transmissionSchema, stateSchema, configSetSimplified,
		producer,
		metrics,
	}
}

type feedMonitor struct {
	log                       logger.Logger
	config                    Config
	feedConfig                FeedConfig
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
	f.metrics.SetFeedContractMetadata(f.config.Solana.ChainID, f.feedConfig.ContractAddress.String(),
		f.feedConfig.ContractStatus, f.feedConfig.ContractType, f.feedConfig.FeedName,
		f.feedConfig.FeedPath, f.config.Solana.NetworkID, f.config.Solana.NetworkName,
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
			err = f.processState(typed)
			if err != nil {
				break
			}
			err = f.processTelemetry(typed)
			latestTransmitter = typed.State.Config.LatestTransmitter.String()
			f.metrics.SetNodeMetadata(f.config.Solana.ChainID, f.config.Solana.NetworkID,
				f.config.Solana.NetworkName, "n/a", latestTransmitter)
			if latestAnswer != nil {
				f.metrics.SetOffchainAggregatorSubmissionReceivedValues(latestAnswer,
					f.feedConfig.ContractAddress.String(), latestTransmitter, f.config.Solana.ChainID,
					f.feedConfig.ContractStatus, f.feedConfig.ContractType, f.feedConfig.FeedName,
					f.feedConfig.FeedPath, f.config.Solana.NetworkID, f.config.Solana.NetworkName)
			}
		case TransmissionEnvelope:
			err = f.processTransmission(typed)
			latestAnswer = typed.Answer.Data
			f.metrics.SetHeadTrackerCurrentHead(typed.BlockNumber, f.config.Solana.NetworkName,
				f.config.Solana.ChainID, f.config.Solana.NetworkID)
			f.metrics.SetOffchainAggregatorAnswers(latestAnswer, f.feedConfig.ContractAddress.String(),
				f.config.Solana.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
				f.feedConfig.FeedName, f.feedConfig.FeedPath, f.config.Solana.NetworkID,
				f.config.Solana.NetworkName)
			f.metrics.IncOffchainAggregatorAnswersTotal(f.feedConfig.ContractAddress.String(),
				f.config.Solana.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
				f.feedConfig.FeedName, f.feedConfig.FeedPath, f.config.Solana.NetworkID,
				f.config.Solana.NetworkName)
			isLateAnswer := time.Since(time.Unix(int64(typed.Answer.Timestamp), 0)).Seconds() > float64(f.feedConfig.HeartbeatSec)
			f.metrics.SetOffchainAggregatorAnswerStalled(isLateAnswer, f.feedConfig.ContractAddress.String(),
				f.config.Solana.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
				f.feedConfig.FeedName, f.feedConfig.FeedPath, f.config.Solana.NetworkID,
				f.config.Solana.NetworkName)
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
	mapping, err := MakeConfigSetMapping(envelope, f.config.Solana, f.feedConfig)
	if err != nil {
		return fmt.Errorf("failed to map message %v: %w", envelope, err)
	}
	value, err := f.stateSchema.Encode(mapping)
	if err != nil {
		return fmt.Errorf("failed to enconde message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value, f.config.ConfigSetTopic); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	return nil
}

func (f *feedMonitor) processTransmission(envelope TransmissionEnvelope) error {
	var mapping map[string]interface{}
	mapping, err := MakeTransmissionMapping(envelope, f.config.Solana, f.feedConfig)
	if err != nil {
		return fmt.Errorf("failed to map message %v: %w", envelope, err)
	}
	value, err := f.transmissionSchema.Encode(mapping)
	if err != nil {
		return fmt.Errorf("failed to enconde message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value, f.config.TransmissionTopic); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	return nil
}

func (f *feedMonitor) processTelemetry(envelope StateEnvelope) error {
	var mapping map[string]interface{}
	mapping, err := MakeTelemetryConfigSetMapping(envelope, f.feedConfig)
	if err != nil {
		return fmt.Errorf("failed to map message %v: %w", envelope, err)
	}
	value, err := f.configSetSimplifiedSchema.Encode(mapping)
	if err != nil {
		return fmt.Errorf("failed to enconde message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value, f.config.ConfigSetSimplifiedTopic); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	return nil
}
