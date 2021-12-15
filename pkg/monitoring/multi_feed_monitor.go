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
	config Config,
	transmissionReader, stateReader AccountReader,
	transmissionSchema, stateSchema, configSetSimplifiedSchema Schema,
	producer Producer,
	metrics Metrics,
) MultiFeedMonitor {
	return &multiFeedMonitor{
		log,
		config,
		transmissionReader, stateReader,
		transmissionSchema, stateSchema, configSetSimplifiedSchema,
		producer,
		metrics,
	}
}

type multiFeedMonitor struct {
	log                       logger.Logger
	config                    Config
	transmissionReader        AccountReader
	stateReader               AccountReader
	transmissionSchema        Schema
	stateSchema               Schema
	configSetSimplifiedSchema Schema
	producer                  Producer
	metrics                   Metrics
}

const bufferCapacity = 100

// Start should be executed as a goroutine.
func (m *multiFeedMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(len(m.config.Feeds))
	for _, feedConfig := range m.config.Feeds {
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
				m.log.With("name", feedConfig.FeedName, "network", m.config.Solana.NetworkName),
				m.config,
				feedConfig,
				transmissionPoller, statePoller,
				m.transmissionSchema, m.stateSchema, m.configSetSimplifiedSchema,
				m.producer,
				m.metrics,
			)
			feedMonitor.Start(ctx)
		}(feedConfig)
	}
}
