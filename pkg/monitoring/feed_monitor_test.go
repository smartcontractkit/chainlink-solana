package monitoring

import (
	"context"
	"testing"
	"time"

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

	transmissionPoller := NewPoller(transmissionAccount, transmissionReader, fetchInterval, bufferCapacity)
	statePoller := NewPoller(stateAccount, stateReader, fetchInterval, bufferCapacity)

	producer := fakeProducer{make(chan producedMessage)}
	telemetryProducer := fakeProducer{make(chan producedMessage)}

	transmissionSchema := fakeSchema{transmissionCodec}
	stateSchema := fakeSchema{configSetCodec}
	telemetrySchema := fakeSchema{telemetryCodec}

	solanaConfig := SolanaConfig{}
	feedConfig := FeedConfig{
		TransmissionsAccount: transmissionAccount,
		StateAccount:         stateAccount,
	}

	monitor := NewFeedMonitor(
		solanaConfig,
		feedConfig,
		transmissionPoller, statePoller,
		transmissionSchema, stateSchema, telemetrySchema,
		producer, telemetryProducer,
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

	// The last update from each poller can potentially be missed by context being cancelled.
	require.GreaterOrEqual(t, len(messages), trCount+stCount-2)
	require.LessOrEqual(t, len(messages), trCount+stCount)
}
