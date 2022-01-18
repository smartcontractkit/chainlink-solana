package monitoring

import (
	"context"
	"sync"

	"github.com/smartcontractkit/chainlink/core/logger"
)

type FeedMonitor interface {
	Start(ctx context.Context, wg *sync.WaitGroup)
}

func NewFeedMonitor(
	log logger.Logger,
	transmissionPoller, statePoller Poller,
	exporters []Exporter,
) FeedMonitor {
	return &feedMonitor{
		log,
		transmissionPoller, statePoller,
		exporters,
	}
}

type feedMonitor struct {
	log                logger.Logger
	transmissionPoller Poller
	statePoller        Poller
	exporters          []Exporter
}

// Start should be executed as a goroutine
func (f *feedMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	f.log.Info("starting feed monitor")
	for {
		// Wait for an update.
		var update interface{}
		select {
		case stateRaw := <-f.statePoller.Updates():
			update = stateRaw
		case answerRaw := <-f.transmissionPoller.Updates():
			update = answerRaw
		case <-ctx.Done():
			return
		}
		wg.Add(len(f.exporters))
		for _, exp := range f.exporters {
			go func(exp Exporter) {
				defer wg.Done()
				exp.Export(ctx, update)
			}(exp)
		}
	}
}
