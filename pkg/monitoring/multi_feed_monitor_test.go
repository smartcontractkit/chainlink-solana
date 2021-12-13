package monitoring

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/require"
)

const numFeeds = 10

func TestMultiFeedMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	cfg := Config{}
	for i := 0; i < numFeeds; i++ {
		feedConfig := generateFeedConfig()
		feedConfig.PollInterval = 5 * time.Second
		cfg.Feeds = append(cfg.Feeds, feedConfig)
	}

	transmissionSchema := fakeSchema{transmissionCodec}
	stateSchema := fakeSchema{configSetCodec}

	producer := fakeProducer{make(chan producerMessage)}

	transmissionReader := &fakeReader{make(chan interface{})}
	stateReader := &fakeReader{make(chan interface{})}

	monitor := NewMultiFeedMonitor(
		logger.NewNullLogger(),
		cfg.Solana,
		transmissionReader, stateReader,
		transmissionSchema, stateSchema,
		producer,
		cfg.Feeds,
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
	require.Equal(t, len(messages), 20)
}
