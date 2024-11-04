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

var errNoComputeUnitPriceCollected = fmt.Errorf("no compute unit prices collected")

type blockHistoryEstimator struct {
	starter services.StateMachine
	chStop  services.StopChan
	done    sync.WaitGroup

	client *utils.LazyLoad[client.ReaderWriter]
	cfg    config.Config
	lgr    logger.Logger

	price uint64
	lock  sync.RWMutex
}

// NewBlockHistoryEstimator creates a new fee estimator that parses historical fees from a fetched block
// Note: getRecentPrioritizationFees is not used because it provides the lowest prioritization fee for an included tx in the block
// which is not effective enough for increasing the chances of block inclusion
func NewBlockHistoryEstimator(c *utils.LazyLoad[client.ReaderWriter], cfg config.Config, lgr logger.Logger) (*blockHistoryEstimator, error) {
	if cfg.BlockHistorySize() < 1 {
		return nil, fmt.Errorf("invalid block history depth: %d", cfg.BlockHistorySize())
	}

	return &blockHistoryEstimator{
		chStop: make(chan struct{}),
		client: c,
		cfg:    cfg,
		lgr:    lgr,
		price:  cfg.ComputeUnitPriceDefault(), // use default value
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
	switch {
	case bhe.cfg.BlockHistorySize() > 1:
		return bhe.calculatePriceFromMultipleBlocks(ctx, bhe.cfg.BlockHistorySize())
	default:
		return bhe.calculatePriceFromLatestBlock(ctx)
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

	if len(feeData.Prices) == 0 {
		return errNoComputeUnitPriceCollected
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
		"computeUnitPrice", v,
		"blockhash", block.Blockhash,
		"slot", block.ParentSlot+1,
		"count", len(feeData.Prices),
	)

	return nil
}

func (bhe *blockHistoryEstimator) calculatePriceFromMultipleBlocks(ctx context.Context, desiredBlockCount uint64) error {
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

	// limit concurrency (avoid hitting rate limits)
	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup
	var mu sync.Mutex
	blockMedians := make([]ComputeUnitPrice, 0, desiredBlockCount)

	// Iterate over the confirmed slots in reverse order to fetch most recent blocks first
	// Iterate until we run out of slots
	for i := len(*confirmedSlots) - 1; i >= 0; i-- {
		slot := (*confirmedSlots)[i]

		wg.Add(1)
		go func(s uint64) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Fetch the block details
			block, errGetBlock := c.GetBlock(ctx, s)
			if errGetBlock != nil {
				bhe.lgr.Errorw("BlockHistoryEstimator: failed to get block at slot", "slot", s, "error", errGetBlock)
				return
			}

			// No block found at slot. Not logging since not all slots may have a block.
			if block == nil {
				return
			}

			// Parse the block to extract compute unit prices
			feeData, errParseBlock := ParseBlock(block)
			if errParseBlock != nil {
				bhe.lgr.Errorw("BlockHistoryEstimator: failed to parse block", "slot", s, "error", errParseBlock)
				return
			}

			// When no relevant transactions for compute unit price are found in this block, we can skip it.
			// No need to log this as an error. It is expected behavior.
			if len(feeData.Prices) == 0 {
				return
			}

			// Calculate the median compute unit price for the block
			blockMedian, errMedian := mathutil.Median(feeData.Prices...)
			if errMedian != nil {
				bhe.lgr.Errorw("BlockHistoryEstimator: failed to calculate median price", "slot", s, "error", errMedian)
				return
			}

			// Append the median compute unit price if we haven't reached our desiredBlockCount
			mu.Lock()
			defer mu.Unlock()
			if uint64(len(blockMedians)) < desiredBlockCount {
				blockMedians = append(blockMedians, blockMedian)
			}
		}(slot)
	}

	wg.Wait()

	if len(blockMedians) == 0 {
		return errNoComputeUnitPriceCollected
	}

	// Calculate avg from medians of the blocks.
	avgOfMedians, err := mathutil.Avg(blockMedians...)
	if err != nil {
		return fmt.Errorf("failed to calculate price from avg of medians: %w", err)
	}

	// Update the current price to the calculated average (avg of medians of the last desiredBlockCount)
	bhe.lock.Lock()
	bhe.price = uint64(avgOfMedians)
	bhe.lock.Unlock()

	bhe.lgr.Debugw("BlockHistoryEstimator: updated",
		"computeUnitPriceMedian", avgOfMedians,
		"latestSlot", currentSlot,
		"numBlocks", len(blockMedians),
		"pricesCollected", blockMedians,
	)

	return nil
}
