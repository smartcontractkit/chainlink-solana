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

	chainConfig := generateChainConfig()
	feedConfig := generateFeedConfig()

	factory := NewRandomDataSourceFactory(ctx, wg, logger.NewNullLogger())
	source, err := factory.NewSource(chainConfig, feedConfig)
	require.NoError(t, err)

	pollInterval := 1 * time.Second
	readTimeout := 1 * time.Second
	var bufferCapacity uint32 = 0 // no buffering

	poller := NewSourcePoller(
		source,
		logger.NewNullLogger(),
		pollInterval, readTimeout,
		bufferCapacity,
	)

	producer := fakeProducer{make(chan producerMessage), ctx}

	transmissionSchema := fakeSchema{transmissionCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}

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

			transmissionSchema,
			configSetSimplifiedSchema,

			cfg.Kafka.TransmissionTopic,
			cfg.Kafka.ConfigSetSimplifiedTopic,
		),
	}

	monitor := NewFeedMonitor(
		logger.NewNullLogger(),
		poller,
		exporters,
	)
	go monitor.Start(ctx, &sync.WaitGroup{})

	count := 0
	var messages []producerMessage
	envelope, err := generateEnvelope()
	require.NoError(t, err)

LOOP:
	for {
		select {
		case factory.updates <- envelope:
			count += 1
			envelope, err = generateEnvelope()
			require.NoError(t, err)
		case message := <-producer.sendCh:
			messages = append(messages, message)
		case <-ctx.Done():
			break LOOP
		}
	}

	// The last update from each poller can potentially be missed by the context being cancelled.
	require.GreaterOrEqual(t, len(messages), 2*count-2)
	require.LessOrEqual(t, len(messages), 2*count)
}
