package monitoring

import (
	"context"
	"sync"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/assert"
)

// Manager restarts the managed function with a new list of updates whenever something changed.
type Manager interface {
	Start(backgroundCtx context.Context, backgroundWg *sync.WaitGroup, managed ManagedFunc)
}

type ManagedFunc func(localCtx context.Context, localWg *sync.WaitGroup, feeds []config.Feed)

func NewManager(
	log logger.Logger,
	rddPoller Poller,
) Manager {
	return &managerImpl{
		log,
		rddPoller,
		[]config.Feed{},
		sync.Mutex{},
	}
}

type managerImpl struct {
	log       logger.Logger
	rddPoller Poller

	currentFeeds   []config.Feed
	currentFeedsMu sync.Mutex
}

func (m *managerImpl) Start(backgroundCtx context.Context, backgroundWg *sync.WaitGroup, managed ManagedFunc) {
	var localCtx context.Context
	var localCtxCancel context.CancelFunc
	var localWg *sync.WaitGroup
	for {
		select {
		case rawUpdatedFeeds := <-m.rddPoller.Updates():
			updatedFeeds, ok := rawUpdatedFeeds.([]config.Feed)
			if !ok {
				m.log.Errorf("unexpected type (%T) for rdd updates", updatedFeeds)
				continue
			}
			shouldRestartMonitor := false
			func() {
				m.currentFeedsMu.Lock()
				defer m.currentFeedsMu.Unlock()
				shouldRestartMonitor = isDifferentFeeds(m.currentFeeds, updatedFeeds)
				if shouldRestartMonitor {
					m.currentFeeds = updatedFeeds
				}
			}()
			if !shouldRestartMonitor {
				continue
			}
			m.log.Infow("change in feeds configuration detected", "feeds", updatedFeeds)
			// Terminate previous managed function if not the first run.
			if localCtxCancel != nil && localWg != nil {
				localCtxCancel()
				localWg.Wait()
			}
			// Start new managed function
			localCtx, localCtxCancel = context.WithCancel(backgroundCtx)
			localWg = new(sync.WaitGroup)
			backgroundWg.Add(1)
			go func() {
				defer backgroundWg.Done()
				managed(localCtx, localWg, updatedFeeds)
			}()
		case <-backgroundCtx.Done():
			if localCtxCancel != nil {
				localCtxCancel()
			}
			if localWg != nil {
				localWg.Wait()
			}
			m.log.Info("manager closed")
			return
		}
	}
}

// isDifferentFeeds checks whether there is a difference between the current list of feeds and the new feeds - Manager
func isDifferentFeeds(current, updated []config.Feed) bool {
	return !assert.ObjectsAreEqual(current, updated)
}
