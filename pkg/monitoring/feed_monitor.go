package monitoring

import (
	"context"
	"fmt"
	"log"
	"time"
)

type FeedMonitor interface {
	Start(ctx context.Context)
}

func NewFeedMonitor(
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
	transmissionPoller, statePoller Poller,
	transmissionSchema, stateSchema Schema,
	producer Producer,
	metrics Metrics,
) FeedMonitor {
	return &feedMonitor{
		solanaConfig,
		feedConfig,
		transmissionPoller, statePoller,
		transmissionSchema, stateSchema,
		producer,
		metrics,
	}
}

type feedMonitor struct {
	solanaConfig       SolanaConfig
	feedConfig         FeedConfig
	transmissionPoller Poller
	statePoller        Poller
	transmissionSchema Schema
	stateSchema        Schema
	producer           Producer
	metrics            Metrics
}

// Start should be executed as a goroutine
func (f *feedMonitor) Start(ctx context.Context) {
	log.Printf("starting feed monitor for feed %v", f.feedConfig)
	f.metrics.SetFeedContractMetadata(f.solanaConfig.ChainID, f.feedConfig.ContractAddress.String(),
		f.feedConfig.ContractStatus, f.feedConfig.ContractType, f.feedConfig.FeedName,
		f.feedConfig.FeedPath, f.solanaConfig.NetworkID, f.solanaConfig.NetworkName,
		f.feedConfig.Symbol)

	go f.transmissionPoller.Start(ctx)
	go f.statePoller.Start(ctx)

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
		case TransmissionEnvelope:
			err = f.processTransmission(typed)
		default:
			err = fmt.Errorf("unknown update type %T", update)
		}
		if err != nil {
			log.Printf("failed to map update %T: %v", update, err)
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
		return fmt.Errorf("failed to enconde message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value); err != nil {
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
		return fmt.Errorf("failed to enconde message %v: %w", envelope, err)
	}
	var key = f.feedConfig.StateAccount.Bytes()
	if err = f.producer.Produce(key, value); err != nil {
		return fmt.Errorf("failed to publish message %v: %w", envelope, err)
	}
	f.metrics.SetHeadTrackerCurrentHead(envelope.BlockNumber, f.solanaConfig.NetworkName,
		f.solanaConfig.ChainID, f.solanaConfig.NetworkID)
	f.metrics.SetOffchainAggregatorAnswers(envelope.Answer.Data, f.feedConfig.ContractAddress.String(),
		f.solanaConfig.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
		f.feedConfig.FeedName, f.feedConfig.FeedPath, f.solanaConfig.NetworkID,
		f.solanaConfig.NetworkName)
	f.metrics.IncOffchainAggregatorAnswersTotal(f.feedConfig.ContractAddress.String(),
		f.solanaConfig.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
		f.feedConfig.FeedName, f.feedConfig.FeedPath, f.solanaConfig.NetworkID,
		f.solanaConfig.NetworkName)
	isLateAnswer := time.Since(time.Unix(int64(envelope.Answer.Timestamp), 0)).Seconds() > float64(f.feedConfig.HeartbeatSec)
	f.metrics.SetOffchainAggregatorAnswerStalled(isLateAnswer, f.feedConfig.ContractAddress.String(),
		f.solanaConfig.ChainID, f.feedConfig.ContractStatus, f.feedConfig.ContractType,
		f.feedConfig.FeedName, f.feedConfig.FeedPath, f.solanaConfig.NetworkID,
		f.solanaConfig.NetworkName)
	return nil
}
