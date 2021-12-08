package monitoring

/*
import (
	"context"
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
		// Serialize the payload.
		mapping, err := TranslateToMap(update)
		if err != nil {
			log.Printf("failed to translate update %#v with error %v", update, err)
			continue
		}
		var key = f.feedConfig.StateAccount.Bytes()
		var value []byte
		switch update.(type) {
		case StateEnvelope:
			value, err = f.stateSchema.Encode(mapping)
		case TransmissionEnvelope:
			value, err = f.transmissionSchema.Encode(mapping)
		}
		if err != nil {
			log.Printf("failed to encode message %v with error: %v", mapping, err)
			continue
		}
		// Push to kafka
		if err = f.producer.Produce(key, value); err != nil {
			log.Printf("failed to publish message %v with error: %v", update, err)
			continue
		}
		// Publish to prometheus
		switch typed := update.(type) {
		case TransmissionEnvelope:
			f.metrics.SetHeadTrackerCurrentHead(typed.BlockNumber, f.solanaConfig.NetworkName,
				f.solanaConfig.ChainID, f.solanaConfig.NetworkID)
			f.metrics.SetOffchainAggregatorAnswers(typed.Answer.Answer, f.feedConfig.ContractAddress.String(),
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
		}
	}
}
*/
