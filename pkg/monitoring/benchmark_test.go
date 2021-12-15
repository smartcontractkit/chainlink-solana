package monitoring

import (
	"context"
	"sync"
	"testing"

	"github.com/smartcontractkit/chainlink/core/logger"
)

// Results:
// goos: darwin
// goarch: amd64
// pkg: github.com/smartcontractkit/chainlink-solana/pkg/monitoring
// cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
// (11 Dec 2021)
//    48993	     35111 ns/op	   44373 B/op	     251 allocs/op
// (13 Dec 2021)
//    47331	     34285 ns/op	   41074 B/op	     235 allocs/op

func BenchmarkMultichainMonitor(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	feed := generateFeedConfig()
	feed.PollInterval = 0 // poll as quickly as possible.
	cfg := Config{}
	cfg.Feeds = []FeedConfig{feed}

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

	transmission := generateTransmissionEnvelope()
	state, err := generateStateEnvelope()
	if err != nil {
		b.Fatalf("failed to generate state: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		select {
		case transmissionReader.readCh <- transmission:
		case stateReader.readCh <- state:
		case <-ctx.Done():
			break
		}
		select {
		case <-producer.sendCh:
		case <-ctx.Done():
			break
		}
	}
}
