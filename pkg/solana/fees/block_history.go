package fees

import (
	"context"
	"fmt"
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

var _ Estimator = &blockHistoryEstimator{}

type EstimationMethod int

const (
	LatestBlockEstimator EstimationMethod = iota
	MultipleBlocksEstimator
)

const maxblockHistoryDepth = 20

func (em EstimationMethod) String() string {
	switch em {
	case LatestBlockEstimator:
		return "LatestBlock"
	case MultipleBlocksEstimator:
		return "MultipleBlocks"
	default:
		return "UnknownMethod"
	}
}

type blockHistoryEstimator struct {
	starter services.StateMachine
	chStop  services.StopChan
	done    sync.WaitGroup

	client *utils.LazyLoad[client.ReaderWriter]
	cfg    config.Config
	lgr    logger.Logger

	price            uint64
	lock             sync.RWMutex
	estimationMethod EstimationMethod
}

// NewBlockHistoryEstimator creates a new fee estimator that parses historical fees from a fetched block
// Note: getRecentPrioritizationFees is not used because it provides the lowest prioritization fee for an included tx in the block
// which is not effective enough for increasing the chances of block inclusion
func NewBlockHistoryEstimator(c *utils.LazyLoad[client.ReaderWriter], cfg config.Config, lgr logger.Logger, method EstimationMethod) (*blockHistoryEstimator, error) {
	return &blockHistoryEstimator{
		chStop:           make(chan struct{}),
		client:           c,
		cfg:              cfg,
		lgr:              lgr,
		price:            cfg.ComputeUnitPriceDefault(), // use default value
		estimationMethod: method,
	}, nil
}

func (bhe *blockHistoryEstimator) Start(ctx context.Context) error {
	return bhe.starter.StartOnce("solana_blockHistoryEstimator", func() error {
		bhe.done.Add(1)
		go bhe.run()
		bhe.lgr.Debugw("BlockHistoryEstimator: started")
		return nil
	})
}

func (bhe *blockHistoryEstimator) run() {
	defer bhe.done.Done()
	ctx, cancel := bhe.chStop.NewCtx()
	defer cancel()

	ticker := services.NewTicker(bhe.cfg.BlockHistoryPollPeriod())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := bhe.calculatePrice(ctx); err != nil {
				bhe.lgr.Error(fmt.Errorf("BlockHistoryEstimator failed to fetch price: %w", err))
			}
		}
	}
}

func (bhe *blockHistoryEstimator) Close() error {
	close(bhe.chStop)
	bhe.done.Wait()
	bhe.lgr.Debugw("BlockHistoryEstimator: stopped")
	return nil
}

func (bhe *blockHistoryEstimator) BaseComputeUnitPrice() uint64 {
	price := bhe.readRawPrice()
	if price >= bhe.cfg.ComputeUnitPriceMin() && price <= bhe.cfg.ComputeUnitPriceMax() {
		return price
	}

	if price < bhe.cfg.ComputeUnitPriceMin() {
		bhe.lgr.Warnw("BlockHistoryEstimator: estimation below minimum consider lowering ComputeUnitPriceMin", "min", bhe.cfg.ComputeUnitPriceMin(), "calculated", price)
		return bhe.cfg.ComputeUnitPriceMin()
	}

	bhe.lgr.Warnw("BlockHistoryEstimator: estimation above maximum consider increasing ComputeUnitPriceMax", "min", bhe.cfg.ComputeUnitPriceMax(), "calculated", price)
	return bhe.cfg.ComputeUnitPriceMax()
}

func (bhe *blockHistoryEstimator) readRawPrice() uint64 {
	bhe.lock.RLock()
	defer bhe.lock.RUnlock()
	return bhe.price
}

func (bhe *blockHistoryEstimator) calculatePrice(ctx context.Context) error {
	switch bhe.estimationMethod {
	case LatestBlockEstimator:
		return bhe.calculatePriceFromLatestBlock(ctx)
	case MultipleBlocksEstimator:
		return bhe.calculatePriceFromMultipleBlocks(ctx)
	default:
		return fmt.Errorf("unknown estimation method")
	}
}

func (bhe *blockHistoryEstimator) calculatePriceFromLatestBlock(ctx context.Context) error {
	// fetch client
	c, err := bhe.client.Get()
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// get latest block based on configured confirmation
	block, err := c.GetLatestBlock(ctx)
	if err != nil {
		return fmt.Errorf("failed to get block: %w", err)
	}

	// parse block for fee data
	feeData, err := ParseBlock(block)
	if err != nil {
		return fmt.Errorf("failed to parse block: %w", err)
	}

	// take median of returned fee values
	v, err := mathutil.Median(feeData.Prices...)
	if err != nil {
		return fmt.Errorf("failed to find median: %w", err)
	}

	// set data
	bhe.lock.Lock()
	bhe.price = uint64(v) // ComputeUnitPrice is uint64 underneath
	bhe.lock.Unlock()
	bhe.lgr.Debugw("BlockHistoryEstimator: updated",
		"method", bhe.estimationMethod.String(),
		"computeUnitPrice", v,
		"blockhash", block.Blockhash,
		"slot", block.ParentSlot+1,
		"count", len(feeData.Prices),
	)

	return nil
}

func (bhe *blockHistoryEstimator) calculatePriceFromMultipleBlocks(ctx context.Context) error {
	// fetch client
	c, err := bhe.client.Get()
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// Fetch the latest slot
	currentSlot, err := c.SlotHeight(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current slot: %w", err)
	}

	desiredBlockCount := bhe.cfg.BlockHistoryDepth()
	if desiredBlockCount == 0 {
		return fmt.Errorf("desiredBlockCount is zero")
	} else if desiredBlockCount > maxblockHistoryDepth {
		// Limit the desired block count to maxblockHistoryDepth
		desiredBlockCount = maxblockHistoryDepth
	}

	allPrices := make([]ComputeUnitPrice, 0, desiredBlockCount)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Determine the starting slot for fetching blocks
	if currentSlot < desiredBlockCount {
		return fmt.Errorf("current slot is less than desired block count")
	}
	startSlot := currentSlot - desiredBlockCount + 1

	// Fetch the last confirmed block slots
	confirmedSlots, err := c.GetBlocksWithLimit(ctx, startSlot, desiredBlockCount)
	if err != nil {
		return fmt.Errorf("failed to get blocks with limit: %w", err)
	}

	// Implement a semaphore to limit concurrency (avoid hitting rate limits)
	semaphore := make(chan struct{}, 10)

	// Iterate over the confirmed slots in reverse order to fetch most recent blocks first
	// Iterate until we have collected the desired number of blocks or we run out of slots
	for i := len(*confirmedSlots) - 1; i >= 0 && uint64(len(allPrices)) < desiredBlockCount; i-- {
		slot := (*confirmedSlots)[i]

		wg.Add(1)
		go func(s uint64) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Fetch the block details
			block, errGetBlock := c.GetBlock(ctx, s)
			if errGetBlock != nil || block == nil {
				// Failed to get block at slot || no block found at slot: skip.
				bhe.lgr.Debugw("get block returned err or nil block", "err", errGetBlock, "slot", s)
				return
			}

			// Parse the block to extract compute unit prices
			feeData, errParseBlock := ParseBlock(block)
			if errParseBlock != nil {
				// Failed to parse block at slot: skip.
				bhe.lgr.Debugw("failed to parse block at slot", "err", errParseBlock, "slot", s)
				return
			}

			// Calculate the median compute unit price for the block
			blockMedian, errMedian := mathutil.Median(feeData.Prices...)
			if errMedian != nil {
				//Failed to calculate median for slot: skip.
				bhe.lgr.Debugw("failed to calculate median for slot", "err", errMedian, "slot", s)
				return
			}

			// Append the median compute unit price
			mu.Lock()
			if uint64(len(allPrices)) < desiredBlockCount {
				allPrices = append(allPrices, blockMedian)
			}
			mu.Unlock()
		}(slot)
	}

	wg.Wait()

	if len(allPrices) == 0 {
		return fmt.Errorf("no compute unit prices collected")
	}

	// Calculate the median of all collected compute unit prices
	medianPrice, err := mathutil.Median(allPrices...)
	if err != nil {
		return fmt.Errorf("failed to calculate median price: %w", err)
	}

	// Update the current price to the median of the last desiredBlockCount
	bhe.lock.Lock()
	bhe.price = uint64(medianPrice)
	bhe.lock.Unlock()

	bhe.lgr.Debugw("BlockHistoryEstimator: updated",
		"method", bhe.estimationMethod.String(),
		"computeUnitPriceMedian", medianPrice,
		"latestSlot", currentSlot,
		"numBlocks", len(allPrices),
		"pricesCollected", allPrices,
	)

	return nil
}
