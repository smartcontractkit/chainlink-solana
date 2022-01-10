package monitoring

import (
	"context"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
)

type Poller interface {
	Start(context.Context)
	// You should never close the channel returned by Updates()!
	// You should always read from the channel returned by Updates() in a select statement with the same context you passed to Start()
	Updates() <-chan interface{}
}

type Source interface {
	Name() string
	Fetch(context.Context) (interface{}, error)
}

func NewSourcePoller(
	source Source,
	log logger.Logger,
	pollInterval time.Duration,
	fetchTimeout time.Duration,
	bufferCapacity uint32,
) Poller {
	return &sourcePoller{
		log.With("source", source.Name()),
		source,
		make(chan interface{}, bufferCapacity),
		pollInterval,
		fetchTimeout,
	}
}

type sourcePoller struct {
	log     logger.Logger
	source  Source
	updates chan interface{}

	pollInterval time.Duration
	fetchTimeout time.Duration
}

// Start should be executed as a goroutine
func (s *sourcePoller) Start(ctx context.Context) {
	s.log.Debug("poller started")
	// Initial fetch.
	data, err := s.source.Fetch(ctx)
	if err != nil {
		s.log.Errorw("failed initial fetch", "error", err)
	} else {
		select {
		case s.updates <- data:
		case <-ctx.Done():
			return
		}
	}

	reusedTimer := time.NewTimer(s.pollInterval)
	for {
		select {
		case <-reusedTimer.C:
			var data interface{}
			var err error
			func() {
				ctx, cancel := context.WithTimeout(ctx, s.fetchTimeout)
				defer cancel()
				data, err = s.source.Fetch(ctx)
			}()
			if err != nil {
				s.log.Errorw("failed to fetch from source", "error", err)
				continue
			}
			select {
			case s.updates <- data:
			case <-ctx.Done():
				return
			}
			reusedTimer.Reset(s.pollInterval)
		case <-ctx.Done():
			if !reusedTimer.Stop() {
				<-reusedTimer.C
			}
			s.log.Debug("poller closed")
			return
		}
	}
}

func (s *sourcePoller) Updates() <-chan interface{} {
	return s.updates
}
