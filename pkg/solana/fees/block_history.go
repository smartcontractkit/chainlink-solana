package fees

import (
	"context"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

var (
	feePolling = 5 * time.Second // TODO: make configurable
)

var _ Estimator = &blockHistoryEstimator{}

type blockHistoryEstimator struct {
	starter services.StateMachine
	chStop  chan struct{}
	done    sync.WaitGroup

	cfg config.Config

	price uint64
	lock  sync.RWMutex
}

// NewBlockHistoryEstimator creates a new fee estimator that parses historical fees from a fetched block
// Note: getRecentPrioritizationFees is not used because it provides the lowest prioritization fee for an included tx in the block
// which is not effective enough for increasing the chances of block inclusion
func NewBlockHistoryEstimator(cfg config.Config) (Estimator, error) {
	return &blockHistoryEstimator{
		chStop: make(chan struct{}),
	}, nil
}

func (bhe *blockHistoryEstimator) Start(ctx context.Context) error {
	return bhe.starter.StartOnce("solana_blockHistoryEstimator", func() error {
		bhe.done.Add(1)
		go bhe.run()
		return nil
	})
}

func (bhe *blockHistoryEstimator) run() {
	defer bhe.done.Done()

	tick := time.After(0)
	for {
		select {
		case <-bhe.chStop:
			return
		case <-tick:
			bhe.lock.Lock()
			bhe.price = 0
			bhe.lock.Unlock()
		}

		tick = time.After(utils.WithJitter(feePolling))
	}
}

func (bhe *blockHistoryEstimator) Close() error {
	close(bhe.chStop)
	bhe.done.Wait()
	return nil
}

func (bhe *blockHistoryEstimator) BaseComputeUnitPrice() uint64 {
	bhe.lock.RLock()
	defer bhe.lock.RUnlock()

	if bhe.price >= bhe.cfg.ComputeUnitPriceMin() && bhe.price <= bhe.cfg.ComputeUnitPriceMax() {
		return bhe.price
	}

	if bhe.price < bhe.cfg.ComputeUnitPriceMin() {
		// TODO: warning log
		return bhe.cfg.ComputeUnitPriceMin()
	}

	// TODO: warning log
	return bhe.cfg.ComputeUnitPriceMax()
}
