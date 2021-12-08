package monitoring

/*
import (
	"context"
	"sync"
	"time"
)

type Service interface {
	Start(ctx context.Context)
}

func NewService(
	solanaConfig SolanaConfig,
	transmissionReader, stateReader AccountReader,
	transmissionSchema, stateSchema Schema,
	producer Producer,
	feeds []FeedConfig,
	metrics Metrics,
) Service {
	return &service{
		solanaConfig,
		transmissionReader, stateReader,
		transmissionSchema, stateSchema,
		producer,
		feeds,
		metrics,
	}
}

type service struct {
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
const pollInterval = 5 * time.Second

// Start should be executed as a goroutine.
func (s *service) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(len(s.feeds))
	for _, feedConfig := range s.feeds {
		go func(feedConfig FeedConfig) {
			defer wg.Done()

			transmissionPoller := NewPoller(feedConfig.TransmissionsAccount, s.transmissionReader, pollInterval, bufferCapacity)
			statePoller := NewPoller(feedConfig.StateAccount, s.stateReader, pollInterval, bufferCapacity)

			feedMonitor := NewFeedMonitor(
				s.solanaConfig, feedConfig,
				transmissionPoller, statePoller,
				s.transmissionSchema, s.stateSchema,
				s.producer, s.metrics,
			)
			feedMonitor.Start(ctx)
		}(feedConfig)
	}
	wg.Wait()
}
*/
