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
	cfg.Solana.PollInterval = 5 * time.Second
	feeds := []config.Feed{}
	for i := 0; i < numFeeds; i++ {
		feeds = append(feeds, generateFeedConfig())
	}

	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}
	transmissionSchema := fakeSchema{transmissionCodec}

	producer := fakeProducer{make(chan producerMessage), ctx}

	transmissionReader := &fakeReader{make(chan interface{})}
	stateReader := &fakeReader{make(chan interface{})}

	monitor := NewMultiFeedMonitor(
		cfg.Solana,

		logger.NewNullLogger(),
		transmissionReader, stateReader,
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

	trCount, stCount := 0, 0
	messages := []producerMessage{}
LOOP:
	for {
		newState, _, _, err := generateState()
		require.NoError(t, err)
		select {
		case transmissionReader.readCh <- generateTransmissionEnvelope():
			trCount += 1
		case stateReader.readCh <- StateEnvelope{newState, 100}:
			stCount += 1
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
	require.Equal(t, 10, stCount, "should only be able to do initial read of the state account")
	require.Equal(t, 20, len(messages))
}

func TestMultiFeedMonitorForPerformance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wg := &sync.WaitGroup{}

	cfg := config.Config{}
	cfg.Solana.PollInterval = 5 * time.Second
	feeds := []config.Feed{}
	for i := 0; i < numFeeds; i++ {
		feeds = append(feeds, generateFeedConfig())
	}

	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}
	transmissionSchema := fakeSchema{transmissionCodec}

	producer := fakeProducer{make(chan producerMessage), ctx}

	transmissionReader := &fakeReader{make(chan interface{})}
	stateReader := &fakeReader{make(chan interface{})}

	monitor := NewMultiFeedMonitor(
		cfg.Solana,

		logger.NewNullLogger(),
		transmissionReader, stateReader,
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

	trCount, stCount := 0, 0
	messages := []producerMessage{}

	wg.Add(1)
	go func() {
		defer wg.Done()
	LOOP:
		for {
			newState, _, _, err := generateState()
			require.NoError(t, err)
			select {
			case transmissionReader.readCh <- generateTransmissionEnvelope():
				trCount += 1
			case stateReader.readCh <- StateEnvelope{newState, 100}:
				stCount += 1
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
	require.Equal(t, 10, trCount, "should only be able to do initial read of the latest transmission")
	require.Equal(t, 10, stCount, "should only be able to do initial read of the state account")
	require.Equal(t, 30, len(messages))
}
