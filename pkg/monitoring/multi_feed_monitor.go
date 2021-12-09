package monitoring

import (
	"context"
	"sync"
)

type MultiFeedMonitor interface {
	Start(ctx context.Context)
}

func NewMultiFeedMonitor(
	solanaConfig SolanaConfig,
	transmissionReader, stateReader AccountReader,
	transmissionSchema, stateSchema Schema,
	producer Producer,
	feeds []FeedConfig,
	metrics Metrics,
) MultiFeedMonitor {
	return &multiFeedMonitor{
		solanaConfig,
		transmissionReader, stateReader,
		transmissionSchema, stateSchema,
		producer,
		feeds,
		metrics,
	}
}

type multiFeedMonitor struct {
	solanaConfig       SolanaConfig
	transmissionReader AccountReader
	stateReader        AccountReader
	transmissionSchema Schema
	stateSchema        Schema
	producer           Producer
	feeds              []FeedConfig
	metrics            Metrics
}

const bufferCapacity = 100

// Start should be executed as a goroutine.
func (m *multiFeedMonitor) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(len(m.feeds))
	for _, feedConfig := range m.feeds {
		go func(feedConfig FeedConfig) {
			defer wg.Done()

			transmissionPoller := NewPoller(feedConfig.TransmissionsAccount, m.transmissionReader, feedConfig.PollInterval, bufferCapacity)
			statePoller := NewPoller(feedConfig.StateAccount, m.stateReader, feedConfig.PollInterval, bufferCapacity)

			feedMonitor := NewFeedMonitor(
				m.solanaConfig,
				feedConfig,
				transmissionPoller, statePoller,
				m.transmissionSchema, m.stateSchema,
				m.producer,
				m.metrics,
			)
			feedMonitor.Start(ctx)
		}(feedConfig)
	}
	wg.Wait()
}
