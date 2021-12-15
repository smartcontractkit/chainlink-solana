package monitoring

import (
	"context"
	"testing"
	"time"

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

	fetchInterval := time.Second
	var bufferCapacity uint32 = 0 // no buffering

	transmissionPoller := NewPoller(logger.NewNullLogger(), transmissionAccount, transmissionReader, fetchInterval, bufferCapacity)
	statePoller := NewPoller(logger.NewNullLogger(), stateAccount, stateReader, fetchInterval, bufferCapacity)

	producer := fakeProducer{make(chan producerMessage)}

	transmissionSchema := fakeSchema{transmissionCodec}
	stateSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetCodec}

	cfg := Config{}

	feedConfig := FeedConfig{
		TransmissionsAccount: transmissionAccount,
		StateAccount:         stateAccount,
	}

	monitor := NewFeedMonitor(
		logger.NewNullLogger(),
		cfg,
		feedConfig,
		transmissionPoller, statePoller,
		transmissionSchema, stateSchema, configSetSimplifiedSchema,
		producer,
		&devnullMetrics{},
	)
	go monitor.Start(ctx)

	trCount, stCount := 0, 0
	messages := []producerMessage{}
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
