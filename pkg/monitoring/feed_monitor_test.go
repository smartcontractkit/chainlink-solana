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
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	chainConfig := generateSolanaConfig()
	feedConfig := generateFeedConfig()

	factory := NewRandomDataSourceFactory(ctx, wg, logger.NewNullLogger())
	sources, err := factory.NewSources(chainConfig, feedConfig)
	require.NoError(t, err)

	pollInterval := 1 * time.Second
	readTimeout := 1 * time.Second
	var bufferCapacity uint32 = 0 // no buffering

	transmissionPoller := NewSourcePoller(
		sources.NewTransmissionsSource(),
		logger.NewNullLogger(),
		pollInterval, readTimeout,
		bufferCapacity,
	)
	statePoller := NewSourcePoller(
		sources.NewConfigSource(),
		logger.NewNullLogger(),
		pollInterval, readTimeout,
		bufferCapacity,
	)

	producer := fakeProducer{make(chan producerMessage), ctx}

	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}
	transmissionSchema := fakeSchema{transmissionCodec}

	cfg := config.Config{}

	exporters := []Exporter{
		NewPrometheusExporter(
			chainConfig,
			feedConfig,
			logger.NewNullLogger(),
			&devnullMetrics{},
		),
		NewKafkaExporter(
			chainConfig,
			feedConfig,
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

	trCount, cfgCount := 0, 0
	var messages []producerMessage
	configEnvelope, err := generateConfigEnvelope()
	require.NoError(t, err)
	transmissionEnvelope := generateTransmissionEnvelope()

LOOP:
	for {
		select {
		case factory.transmissions <- transmissionEnvelope:
			trCount += 1
			transmissionEnvelope = generateTransmissionEnvelope()
		case factory.configs <- configEnvelope:
			cfgCount += 1
			configEnvelope, err = generateConfigEnvelope()
			require.NoError(t, err)
		case message := <-producer.sendCh:
			messages = append(messages, message)
		case <-ctx.Done():
			break LOOP
		}
	}

	// The last update from each poller can potentially be missed by context being cancelled.
	require.GreaterOrEqual(t, len(messages), trCount+cfgCount-2)
	require.LessOrEqual(t, len(messages), trCount+cfgCount)
}
