package monitoring

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/require"
)

func TestFeedMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	transmissionReader := &fakeReader{make(chan interface{})}
	stateReader := &fakeReader{make(chan interface{})}

	transmissionAccount := generatePublicKey()
	stateAccount := generatePublicKey()

	pollInterval := 1 * time.Second
	readTimeout := 1 * time.Second
	var bufferCapacity uint32 = 0 // no buffering

	transmissionPoller := NewPoller(
		logger.NewNullLogger(),
		transmissionAccount, transmissionReader,
		pollInterval, readTimeout,
		bufferCapacity,
	)
	statePoller := NewPoller(
		logger.NewNullLogger(),
		stateAccount, stateReader,
		pollInterval, readTimeout,
		bufferCapacity,
	)

	producer := fakeProducer{make(chan producerMessage), ctx}

	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}
	transmissionSchema := fakeSchema{transmissionCodec}

	cfg := config.Config{}
	cfg.Feeds.Feeds = []config.Feed{
		{
			TransmissionsAccount: transmissionAccount,
			StateAccount:         stateAccount,
		},
	}

	exporters := []Exporter{
		NewPrometheusExporter(
			cfg.Solana,
			cfg.Feeds.Feeds[0],
			logger.NewNullLogger(),
			&devnullMetrics{},
		),
		NewKafkaExporter(
			cfg.Solana,
			cfg.Feeds.Feeds[0],
			logger.NewNullLogger(),
			producer,

			configSetSchema,
			configSetSimplifiedSchema,
			transmissionSchema,

			cfg.Kafka.ConfigSetTopic,
			cfg.Kafka.ConfigSetSimplifiedTopic,
			cfg.Kafka.TransmissionTopic,
		),
	}

	monitor := NewFeedMonitor(
		logger.NewNullLogger(),
		transmissionPoller, statePoller,
		exporters,
	)
	go monitor.Start(ctx, &sync.WaitGroup{})

	trCount, stCount := 0, 0
	var messages []producerMessage
	newStateEnv, err := generateStateEnvelope()
	require.NoError(t, err)
	newTransmissionEnv := generateTransmissionEnvelope()

LOOP:
	for {
		select {
		case transmissionReader.readCh <- newTransmissionEnv:
			trCount += 1
			newTransmissionEnv = generateTransmissionEnvelope()
		case stateReader.readCh <- newStateEnv:
			stCount += 1
			newStateEnv, err = generateStateEnvelope()
			require.NoError(t, err)
		case message := <-producer.sendCh:
			messages = append(messages, message)
		case <-ctx.Done():
			break LOOP
		}
	}

	// The last update from each poller can potentially be missed by context being cancelled.
	require.GreaterOrEqual(t, len(messages), trCount+stCount-2)
	require.LessOrEqual(t, len(messages), trCount+stCount)
}
