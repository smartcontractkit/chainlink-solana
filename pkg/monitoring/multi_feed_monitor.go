package monitoring

import (
	"context"
	"sync"

	"github.com/smartcontractkit/chainlink/core/logger"
)

type MultiFeedMonitor interface {
	Start(ctx context.Context, wg *sync.WaitGroup)
}

func NewMultiFeedMonitor(
	log logger.Logger,
	solanaConfig SolanaConfig,
	transmissionReader, stateReader AccountReader,
	transmissionSchema, stateSchema Schema,
	producer Producer,
	feeds []FeedConfig,
	metrics Metrics,
) MultiFeedMonitor {
	return &multiFeedMonitor{
		log,
		solanaConfig,
		transmissionReader, stateReader,
		transmissionSchema, stateSchema,
		producer,
		feeds,
		metrics,
	}
}

type multiFeedMonitor struct {
	log                logger.Logger
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
func (m *multiFeedMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(len(m.feeds))
	for _, feedConfig := range m.feeds {
		go func(feedConfig FeedConfig) {
			defer wg.Done()

			transmissionPoller := NewPoller(
				m.log.With("account", "transmissions", "address", feedConfig.TransmissionsAccount.String()),
				feedConfig.TransmissionsAccount,
				m.transmissionReader,
				feedConfig.PollInterval,
				bufferCapacity,
			)
			statePoller := NewPoller(
				m.log.With("account", "state", "address", feedConfig.StateAccount.String()),
				feedConfig.StateAccount,
				m.stateReader,
				feedConfig.PollInterval,
				bufferCapacity,
			)

			wg.Add(2)
			go func() {
				defer wg.Done()
				transmissionPoller.Start(ctx)
			}()
			go func() {
				defer wg.Done()
				statePoller.Start(ctx)
			}()

			feedMonitor := NewFeedMonitor(
				m.log.With("name", feedConfig.FeedName, "network", m.solanaConfig.NetworkName),
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
}
