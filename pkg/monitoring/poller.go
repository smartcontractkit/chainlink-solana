package monitoring

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink/core/logger"
)

type Poller interface {
	Start(context.Context)
	Updates() <-chan interface{}
}

func NewPoller(
	log logger.Logger,
	account solana.PublicKey,
	reader AccountReader,
	pollInterval time.Duration,
	readTimeout time.Duration,
	bufferCapacity uint32,
) Poller {
	return &solanaPollerImpl{
		log,
		account,
		reader,
		pollInterval,
		readTimeout,
		bufferCapacity,
		make(chan interface{}, bufferCapacity),
	}
}

type solanaPollerImpl struct {
	log            logger.Logger
	account        solana.PublicKey
	reader         AccountReader
	pollInterval   time.Duration
	readTimeout    time.Duration
	bufferCapacity uint32
	updates        chan interface{}
}

// Start should be executed as a goroutine
func (s *solanaPollerImpl) Start(ctx context.Context) {
	s.log.Debug("poller started")
	// Fetch initial data
	data, err := s.reader.Read(ctx, s.account)
	if err != nil {
		s.log.Errorw("failed initial fetch of account contents", "error", err)
	} else {
		s.updates <- data
	}

	for {
		timer := time.NewTimer(s.pollInterval)
		select {
		case <-timer.C:
			var data interface{}
			var err error
			func() {
				ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
				defer cancel()
				data, err = s.reader.Read(ctx, s.account)
			}()
			if err != nil {
				s.log.Errorw("failed to read account contents", "error", err)
				continue
			}
			s.updates <- data
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			s.log.Debug("poller closed")
			return
		}
	}
}

func (s *solanaPollerImpl) Updates() <-chan interface{} {
	return s.updates
}
