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

func TestMultiFeedMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wg := &sync.WaitGroup{}

	cfg := config.Config{}
	cfg.Solana.PollInterval = 5 * time.Second
	feeds := []config.Feed{}
	for i := 0; i < numFeeds; i++ {
		feeds = append(feeds, generateFeedConfig())
	}

	transmissionSchema := fakeSchema{transmissionCodec}
	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetSimplifiedCodec}

	producer := fakeProducer{make(chan producerMessage)}

	transmissionReader := &fakeReader{make(chan interface{})}
	stateReader := &fakeReader{make(chan interface{})}

	monitor := NewMultiFeedMonitor(
		logger.NewNullLogger(),
		cfg.Solana,
		feeds,
		cfg.Kafka.ConfigSetTopic, cfg.Kafka.ConfigSetSimplifiedTopic, cfg.Kafka.TransmissionTopic,
		transmissionReader, stateReader,
		transmissionSchema, configSetSchema, configSetSimplifiedSchema,
		producer,
		&devnullMetrics{},
	)
	go monitor.Start(ctx, wg)

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
		case message := <-producer.sendCh:
			messages = append(messages, message)
		case <-ctx.Done():
			break LOOP
		}
	}

	wg.Wait()
	require.Equal(t, trCount, 10, "should only be able to do initial read of the latest transmission")
	require.Equal(t, stCount, 10, "should only be able to do initial read of the state account")
	require.Equal(t, len(messages), 30)
}
