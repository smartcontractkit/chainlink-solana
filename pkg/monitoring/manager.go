package monitoring

import (
	"context"
	"sync"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/assert"
)

// Manager restarts the multi feed monitor with a new list of feeds whenever something changed.
type Manager interface {
	Start(ctx context.Context, wg *sync.WaitGroup, callback ManagerCallback)
}

type ManagerCallback func(ctx context.Context, wg *sync.WaitGroup, feeds []config.Feed)

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

func (m *managerImpl) Start(ctx context.Context, _ *sync.WaitGroup, callback ManagerCallback) {
	localCtx, localCtxCancel := context.WithCancel(ctx)
	defer localCtxCancel()
	localWg := new(sync.WaitGroup)
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
			// Terminate previous callback.
			localCtxCancel()
			localWg.Wait()

			// Start new callback.
			localCtx, localCtxCancel = context.WithCancel(ctx)
			localWg = new(sync.WaitGroup)
			localWg.Add(1)
			go func() {
				defer localWg.Done()
				callback(localCtx, localWg, updatedFeeds)
			}()
		case <-ctx.Done():
			return
		}
	}
}

// isDifferentFeeds checks whether there is a difference between the current list of feeds and the new feeds - Manager
func isDifferentFeeds(current, updated []config.Feed) bool {
	return assert.ObjectsAreEqual(current, updated)
}
