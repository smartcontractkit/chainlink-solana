package monitoring

import (
	"context"
	"sync"
	"testing"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
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
// (3 Jan 2022)
//    6985	    162187 ns/op	  114802 B/op	    1506 allocs/op
// (4 Jan 2022)
//    9332	    166275 ns/op	  157078 B/op	    1590 allocs/op
// (17 Jan 2022)
//    7374	    202079 ns/op	  164301 B/op	    1712 allocs/op

func BenchmarkMultichainMonitorStatePath(b *testing.B) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{}
	chainCfg := SolanaConfig{}
	feeds := []FeedConfig{generateFeedConfig()}

	transmissionSchema := fakeSchema{transmissionCodec}
	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetCodec}

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

	configEnvelope, err := generateConfigEnvelope()
	if err != nil {
		b.Fatalf("failed to generate config: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		select {
		case factory.configs <- configEnvelope:
		case <-ctx.Done():
			continue
		}
		select {
		case <-producer.sendCh:
		case <-ctx.Done():
			continue
		}
	}
}

// Results:
// goos: darwin
// goarch: amd64
// pkg: github.com/smartcontractkit/chainlink-solana/pkg/monitoring
// cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
// (4 Jan 2022)
//    61338	     18841 ns/op	    6606 B/op	     137 allocs/op
// (17 Jan 2022)
//    56856	     20759 ns/op	    6866 B/op	     141 allocs/op

func BenchmarkMultichainMonitorTransmissionPath(b *testing.B) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Config{}
	chainCfg := SolanaConfig{}
	feeds := []FeedConfig{generateFeedConfig()}

	transmissionSchema := fakeSchema{transmissionCodec}
	configSetSchema := fakeSchema{configSetCodec}
	configSetSimplifiedSchema := fakeSchema{configSetCodec}

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

	transmissionEnvelope := generateTransmissionEnvelope()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		select {
		case factory.transmissions <- transmissionEnvelope:
		case <-ctx.Done():
			continue
		}
		select {
		case <-producer.sendCh:
		case <-ctx.Done():
			continue
		}
	}
}
