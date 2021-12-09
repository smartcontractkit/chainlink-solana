package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const numFeeds = 10

func TestSmokeTest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := Config{}
	for i := 0; i < numFeeds; i++ {
		feedConfig := generateFeedConfig()
		feedConfig.PollInterval = 5 * time.Second
		cfg.Feeds = append(cfg.Feeds, feedConfig)
	}

	transmissionSchema := fakeSchema{transmissionCodec}
	stateSchema := fakeSchema{configSetCodec}

	producer := fakeProducer{make(chan producedMessage)}

	transmissionReader := &fakeReader{make(chan interface{})}
	stateReader := &fakeReader{make(chan interface{})}

	monitor := NewMultiFeedMonitor(
		cfg.Solana,
		transmissionReader, stateReader,
		transmissionSchema, stateSchema,
		producer,
		cfg.Feeds,
		&devnullMetrics{},
	)
	go monitor.Start(ctx)

	trCount, stCount := 0, 0
	messages := []producedMessage{}
LOOP:
	for {
		newState, _, _ := generateState()
		select {
		case transmissionReader.readCh <- generateTransmissionEnvelope(trCount):
			trCount += 1
		case stateReader.readCh <- StateEnvelope{newState, 100}:
			stCount += 1
		case message := <-producer.sendCh:
			messages = append(messages, message)
		case <-ctx.Done():
			break LOOP
		}
	}

	require.Equal(t, trCount, 10, "should only be able to do initial read of the latest transmission")
	require.Equal(t, stCount, 10, "should only be able to do initial read of the state account")
	require.Equal(t, len(messages), 20)
}
