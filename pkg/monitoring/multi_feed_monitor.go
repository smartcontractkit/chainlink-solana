package monitoring

import (
	"context"
	"sync"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
)

type MultiFeedMonitor interface {
	Start(ctx context.Context, wg *sync.WaitGroup)
}

func NewMultiFeedMonitor(
	log logger.Logger,
	solanaConfig config.Solana,
	feeds []config.Feed,
	configSetTopic, configSetSimplifiedTopic, transmissionTopic string,
	transmissionReader, stateReader AccountReader,
	transmissionSchema, configSetSchema, configSetSimplifiedSchema Schema,
	producer Producer,
	metrics Metrics,
) MultiFeedMonitor {
	return &multiFeedMonitor{
		log,
		solanaConfig,
		feeds,
		configSetTopic, configSetSimplifiedTopic, transmissionTopic,
		transmissionReader, stateReader,
		transmissionSchema, configSetSchema, configSetSimplifiedSchema,
		producer,
		metrics,
	}
}

type multiFeedMonitor struct {
	log                       logger.Logger
	solanaConfig              config.Solana
	feeds                     []config.Feed
	configSetTopic            string
	configSetSimplifiedTopic  string
	transmissionTopic         string
	transmissionReader        AccountReader
	stateReader               AccountReader
	transmissionSchema        Schema
	configSetSchema           Schema
	configSetSimplifiedSchema Schema
	producer                  Producer
	metrics                   Metrics
}

const bufferCapacity = 100

// Start should be executed as a goroutine.
func (m *multiFeedMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(len(m.feeds))
	for _, feedConfig := range m.feeds {
		go func(feedConfig config.Feed) {
			defer wg.Done()

			transmissionPoller := NewPoller(
				m.log.With("account", "transmissions", "address", feedConfig.TransmissionsAccount.String()),
				feedConfig.TransmissionsAccount,
				m.transmissionReader,
				m.solanaConfig.PollInterval,
				m.solanaConfig.ReadTimeout,
				bufferCapacity,
			)
			statePoller := NewPoller(
				m.log.With("account", "state", "address", feedConfig.StateAccount.String()),
				feedConfig.StateAccount,
				m.stateReader,
				m.solanaConfig.PollInterval,
				m.solanaConfig.ReadTimeout,
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
				m.log.With("feed", feedConfig.FeedName, "network", m.solanaConfig.NetworkName),
				m.solanaConfig,
				feedConfig,
				m.configSetTopic, m.configSetSimplifiedTopic, m.transmissionTopic,
				transmissionPoller, statePoller,
				m.transmissionSchema, m.configSetSchema, m.configSetSimplifiedSchema,
				m.producer,
				m.metrics,
			)
			feedMonitor.Start(ctx)
		}(feedConfig)
	}
}
