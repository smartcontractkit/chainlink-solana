package fees

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

var (
	promBHEComputeUnitPrice = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "solana_bhe_compute_unit_price",
		Help: "The compute unit price determined by the Solana block history estimator",
	}, []string{"chainID"})
)

var _ Estimator = &blockHistoryEstimator{}

var errNoComputeUnitPriceCollected = fmt.Errorf("no compute unit prices collected")

type blockHistoryEstimator struct {
	starter services.StateMachine
	chStop  services.StopChan
	done    sync.WaitGroup

	client func(context.Context) (client.ReaderWriter, error)
	cfg    config.Config
	lgr    logger.Logger

	price uint64
	lock  sync.RWMutex

	cache   blockMedianCache
	cacheMu sync.RWMutex

	// metrics
	computeUnitPrice metric.Float64Gauge
	chainID          string
}

type blockMedianCache struct {
	storedBlockRange []uint64
	medianMap        map[uint64]ComputeUnitPrice // block num to median
}

// NewBlockHistoryEstimator creates a new fee estimator that parses historical fees from a fetched block
// Note: getRecentPrioritizationFees is not used because it provides the lowest prioritization fee for an included tx in the block
// which is not effective enough for increasing the chances of block inclusion
func NewBlockHistoryEstimator(c func(context.Context) (client.ReaderWriter, error), cfg config.Config, lgr logger.Logger, chainID string) (*blockHistoryEstimator, error) {
	if cfg.BlockHistorySize() < 1 {
		return nil, fmt.Errorf("invalid block history depth: %d", cfg.BlockHistorySize())
	}

	computeUnitPrice, err := beholder.GetMeter().Float64Gauge("solana_bhe_compute_unit_price")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana block history estimator average compute unit price metric: %w", err)
	}

	return &blockHistoryEstimator{
		chStop:           make(chan struct{}),
		client:           c,
		cfg:              cfg,
		lgr:              lgr,
		price:            cfg.ComputeUnitPriceDefault(), // use default value
		cache:            blockMedianCache{storedBlockRange: make([]uint64, 0, cfg.BlockHistorySize()), medianMap: make(map[uint64]ComputeUnitPrice, cfg.BlockHistorySize())},
		computeUnitPrice: computeUnitPrice,
		chainID:          chainID,
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
		bhe.lgr.Debugw("BlockHistoryEstimator: estimation below minimum consider lowering ComputeUnitPriceMin", "min", bhe.cfg.ComputeUnitPriceMin(), "calculated", price)
		return bhe.cfg.ComputeUnitPriceMin()
	}

	bhe.lgr.Warnw("BlockHistoryEstimator: estimation above maximum consider increasing ComputeUnitPriceMax", "max", bhe.cfg.ComputeUnitPriceMax(), "calculated", price)
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
		if err := bhe.populateCache(ctx, bhe.cfg.BlockHistoryBatchLoadSize(), bhe.cfg.BlockHistorySize()); err != nil {
			return fmt.Errorf("failed to populate cache: %w", err)
		}
		return bhe.calculatePriceFromMultipleBlocks(ctx, bhe.cfg.BlockHistorySize())
	default:
		return bhe.calculatePriceFromLatestBlock(ctx)
	}
}

func (bhe *blockHistoryEstimator) calculatePriceFromLatestBlock(ctx context.Context) error {
	// fetch client
	c, err := bhe.client(ctx)
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

	// Record the compute unit price for prometheus and beholder metrics
	bhe.recordComputeUnitPrice(ctx, v)

	return nil
}

func (bhe *blockHistoryEstimator) calculatePriceFromMultipleBlocks(ctx context.Context, desiredBlockCount uint64) error {
	bhe.cacheMu.RLock()
	defer bhe.cacheMu.RUnlock()
	blockMedians := make([]ComputeUnitPrice, 0, desiredBlockCount)

	if len(bhe.cache.medianMap) == 0 {
		return errNoComputeUnitPriceCollected
	}

	for _, median := range bhe.cache.medianMap {
		blockMedians = append(blockMedians, median)
	}

	// Calculate avg from medians of the blocks.
	avgOfMedians, err := mathutil.Avg(blockMedians...)
	if err != nil {
		return fmt.Errorf("failed to calculate price from avg of medians: %w", err)
	}

	// Update the current price to the calculated average
	// The calculated average could be over a smaller range of blocks than desiredBlockCount if cache is partially filled during startup
	bhe.lock.Lock()
	bhe.price = uint64(avgOfMedians)
	bhe.lock.Unlock()

	bhe.lgr.Debugw("BlockHistoryEstimator: updated",
		"computeUnitPriceAvg", avgOfMedians,
		"numBlocks", len(blockMedians),
	)

	// Record the compute unit price for prometheus and beholder metrics
	bhe.recordComputeUnitPrice(ctx, avgOfMedians)

	return nil
}

func (bhe *blockHistoryEstimator) populateCache(ctx context.Context, loadBatch, desiredBlockCount uint64) error {
	batch := uint64(math.Min(float64(loadBatch), float64(desiredBlockCount)))
	// fetch client
	c, err := bhe.client(ctx)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// Fetch the latest slot for processed commitment
	currentSlot, err := c.SlotHeight(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current slot: %w", err)
	}

	// Determine the starting slot for fetching blocks
	if currentSlot < batch {
		return fmt.Errorf("current slot is less than desired block count")
	}
	startSlot := currentSlot - batch + 1

	// Fetch the latest slots with blocks for the configured commitment level
	confirmedSlots, err := c.GetBlocksWithLimit(ctx, startSlot, batch)
	if err != nil {
		return fmt.Errorf("failed to get blocks with limit: %w", err)
	}

	// limit concurrency (avoid hitting rate limits)
	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup

	// Iterate over the confirmed slots in reverse order to fetch most recent blocks first
	// Iterate until we run out of slots
	for i := len(*confirmedSlots) - 1; i >= 0; i-- {
		slot := (*confirmedSlots)[i]

		// If median already exists for slot, skip fetching block
		bhe.cacheMu.RLock()
		_, exists := bhe.cache.medianMap[slot]
		bhe.cacheMu.RUnlock()
		if exists {
			continue
		}

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

			// Store the block median price in cache
			bhe.cacheMu.Lock()
			bhe.cache.medianMap[s] = blockMedian
			bhe.cache.storedBlockRange = append(bhe.cache.storedBlockRange, s)
			bhe.cacheMu.Unlock()
		}(slot)
	}

	wg.Wait()

	bhe.cacheMu.Lock()
	defer bhe.cacheMu.Unlock()
	excessBlocks := len(bhe.cache.storedBlockRange) - int(desiredBlockCount) //nolint:gosec // block history size cannot reasonably exceed int max
	// Return early if cache size does not exceed the desired block count
	if excessBlocks <= 0 {
		return nil
	}
	// Sort stored block nums (oldest to newest)
	slices.Sort(bhe.cache.storedBlockRange)
	// Clear out the oldest blocks that exceed the desired block count
	for i := range excessBlocks {
		slot := bhe.cache.storedBlockRange[i]
		delete(bhe.cache.medianMap, slot)
	}
	bhe.cache.storedBlockRange = bhe.cache.storedBlockRange[excessBlocks:]
	return nil
}

func (bhe *blockHistoryEstimator) recordComputeUnitPrice(ctx context.Context, avgOfMedians ComputeUnitPrice) {
	promBHEComputeUnitPrice.WithLabelValues(bhe.chainID).Set(float64(avgOfMedians))
	bhe.computeUnitPrice.Record(ctx, float64(avgOfMedians), metric.WithAttributes(attribute.String("chainID", bhe.chainID)))
}
