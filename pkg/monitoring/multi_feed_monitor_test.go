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

const numFeeds = 10

func TestMultiFeedMonitorToMakeSureAllGoroutinesTerminate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wg := &sync.WaitGroup{}

	cfg := config.Config{}
	solanaCfg := SolanaConfig{
		PollInterval: 5 * time.Second,
	}
	feeds := []FeedConfig{}
	for i := 0; i < numFeeds; i++ {
		feeds = append(feeds, generateFeedConfig())
	}

	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}
	transmissionSchema := fakeSchema{transmissionCodec}

	producer := fakeProducer{make(chan producerMessage), ctx}

	factory := &fakeRandomDataSourceFactory{
		make(chan TransmissionEnvelope),
		make(chan ConfigEnvelope),
	}

	monitor := NewMultiFeedMonitor(
		solanaCfg,

		logger.NewNullLogger(),
		factory,
		producer,
		&devnullMetrics{},

		cfg.Kafka.ConfigSetTopic,
		cfg.Kafka.ConfigSetSimplifiedTopic,
		cfg.Kafka.TransmissionTopic,

		configSetSchema,
		configSetSimplifiedSchema,
		transmissionSchema,
	)
	go monitor.Start(ctx, wg, feeds)

	trCount, cfgCount := 0, 0
	messages := []producerMessage{}

	configEnvelope, err := generateConfigEnvelope()
	require.NoError(t, err)

LOOP:
	for {
		select {
		case factory.transmissions <- generateTransmissionEnvelope():
			trCount += 1
		case factory.configs <- configEnvelope:
			cfgCount += 1
			configEnvelope, err = generateConfigEnvelope()
			require.NoError(t, err)
		case <-ctx.Done():
			break LOOP
		}
		select {
		case message := <-producer.sendCh:
			messages = append(messages, message)
		case <-ctx.Done():
			break LOOP
		}
	}

	wg.Wait()
	require.Equal(t, 10, trCount, "should only be able to do initial read of the latest transmission")
	require.Equal(t, 10, cfgCount, "should only be able to do initial read of the state account")
	require.Equal(t, 20, len(messages))
}

func TestMultiFeedMonitorForPerformance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wg := &sync.WaitGroup{}

	cfg := config.Config{}
	chainCfg := SolanaConfig{
		PollInterval: 5 * time.Second,
	}
	feeds := []FeedConfig{}
	for i := 0; i < numFeeds; i++ {
		feeds = append(feeds, generateFeedConfig())
	}

	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}
	transmissionSchema := fakeSchema{transmissionCodec}

	producer := fakeProducer{make(chan producerMessage), ctx}

	factory := &fakeRandomDataSourceFactory{
		make(chan TransmissionEnvelope),
		make(chan ConfigEnvelope),
	}

	monitor := NewMultiFeedMonitor(
		chainCfg,

		logger.NewNullLogger(),
		factory,
		producer,
		&devnullMetrics{},

		cfg.Kafka.ConfigSetTopic,
		cfg.Kafka.ConfigSetSimplifiedTopic,
		cfg.Kafka.TransmissionTopic,

		configSetSchema,
		configSetSimplifiedSchema,
		transmissionSchema,
	)
	go monitor.Start(ctx, wg, feeds)

	var trCount, cfgCount int64 = 0, 0
	messages := []producerMessage{}

	configEnvelope, err := generateConfigEnvelope()
	require.NoError(t, err)

	wg.Add(1)
	go func() {
		defer wg.Done()
	LOOP:
		for {
			select {
			case factory.transmissions <- generateTransmissionEnvelope():
				trCount += 1
			case factory.configs <- configEnvelope:
				cfgCount += 1
				configEnvelope, err = generateConfigEnvelope()
				require.NoError(t, err)
			case <-ctx.Done():
				break LOOP
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
	LOOP:
		for {
			select {
			case message := <-producer.sendCh:
				messages = append(messages, message)
			case <-ctx.Done():
				break LOOP
			}
		}
	}()

	wg.Wait()
	require.Equal(t, int64(10), trCount, "should only be able to do initial read of the latest transmission")
	require.Equal(t, int64(10), cfgCount, "should only be able to do initial read of the state account")
	require.Equal(t, 20, len(messages))
}
