package fees

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	cfgmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
)

func TestBlockHistoryEstimator_InvalidBlockHistorySize(t *testing.T) {
	// Setup
	invalidDepth := uint64(0) // Invalid value to trigger error
	rw := clientmock.NewReaderWriter(t)
	rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
	cfg := cfgmock.NewConfig(t)
	cfg.On("BlockHistorySize").Return(invalidDepth)

	// Initialize estimator and expect an error due to invalid BlockHistorySize
	_, err := NewBlockHistoryEstimator(rwLoader, cfg, logger.Test(t), "")
	require.Error(t, err, "Expected error for invalid block history size")
	assert.Equal(t, "invalid block history depth: 0", err.Error(), "Unexpected error message for invalid block history size")
}

func TestBlockHistoryEstimator_LatestBlock(t *testing.T) {
	// Helper variables for tests
	minPrice := uint64(10)
	maxPrice := uint64(100_000)
	defaultPrice := uint64(100)
	depth := uint64(1) // 1 is LatestBlockEstimator
	pollPeriod := 100 * time.Millisecond
	ctx := t.Context()
	chainID := "chainID"

	// Grabbing last block of multiple_blocks file to use as latest block
	testBlocks := readMultipleBlocksFromFile(t, "./multiple_blocks_data.json")
	lastBlock := testBlocks[len(testBlocks)-1]
	lastBlockFeeData, _ := ParseBlock(lastBlock)
	lastBlockMedianPrice, _ := mathutil.Median(lastBlockFeeData.Prices...)

	rw := clientmock.NewReaderWriter(t)
	rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
	rw.On("GetLatestBlock", mock.Anything).Return(lastBlock, nil)

	t.Run("Successful Estimation", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Assert the computed price matches the expected price
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price")
		cfg.On("ComputeUnitPriceMin").Return(minPrice)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		assert.Equal(t, uint64(lastBlockMedianPrice), estimator.BaseComputeUnitPrice())
		assert.Equal(t, float64(lastBlockMedianPrice), testutil.ToFloat64(promBHEComputeUnitPrice.WithLabelValues(chainID)), "metric did not record compute unit price")
	})

	t.Run("Min Gate: Price Should Be Floored at Min", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMin := uint64(lastBlockMedianPrice) + 100 // Set min higher than the median price
		setupConfigMock(cfg, defaultPrice, tmpMin, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Call calculatePrice and ensure no error
		// Assert the compute unit price is floored at min
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price with price below min")
		cfg.On("ComputeUnitPriceMin").Return(tmpMin)
		assert.Equal(t, tmpMin, estimator.BaseComputeUnitPrice(), "Price should be floored at min")
	})

	t.Run("Max Gate: Price Should Be Capped at Max", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMax := uint64(lastBlockMedianPrice) - 100 // Set max lower than the median price
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Call calculatePrice and ensure no error
		// Assert the compute unit price is capped at max
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price with price above max")
		cfg.On("ComputeUnitPriceMax").Return(tmpMax)
		cfg.On("ComputeUnitPriceMin").Return(minPrice)
		assert.Equal(t, tmpMax, estimator.BaseComputeUnitPrice(), "Price should be capped at max")
	})

	t.Run("Failed to Get Latest Block", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		rw.On("GetLatestBlock", mock.Anything).Return(nil, fmt.Errorf("fail rpc call")) // Mock GetLatestBlock returning error
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Ensure the price remains unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when GetLatestBlock fails")
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice(), "Price should not change when GetLatestBlock fails")
	})

	t.Run("Failed to Parse Block", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		rw.On("GetLatestBlock", mock.Anything).Return(nil, nil) // Mock GetLatestBlock returning nil
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Ensure the price remains unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when parsing fails")
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice(), "Price should not change when parsing fails")
	})

	t.Run("no compute unit prices collected", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		rw.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil) // Mock GetLatestBlock returning empty block
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Ensure the price remains unchanged
		require.EqualError(t, estimator.calculatePrice(ctx), errNoComputeUnitPriceCollected.Error(), "Expected error when no compute unit prices are collected")
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice(), "Price should not change when median calculation fails")
	})

	t.Run("Failed to Get Client", func(t *testing.T) {
		// Setup
		rwFailLoader := func(ctx context.Context) (client.ReaderWriter, error) {
			// Return error to simulate failure to get client
			return nil, fmt.Errorf("fail client load")
		}
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwFailLoader, cfg, logger.Test(t), chainID)

		// Call calculatePrice and expect an error
		// Ensure the price remains unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when getting client fails")
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice(), "Price should remain at default when client fails")
	})
}

func TestBlockHistoryEstimator_MultipleBlocks(t *testing.T) {
	// helpers vars for tests
	minPrice := uint64(100)
	maxPrice := uint64(100_000)
	depth := uint64(3)
	defaultPrice := uint64(100)
	pollPeriod := 3 * time.Second
	ctx := t.Context()
	chainID := "chainID"

	// Read multiple blocks from JSON file
	testBlocks := readMultipleBlocksFromFile(t, "./multiple_blocks_data.json")
	require.GreaterOrEqual(t, len(testBlocks), int(depth), "Not enough blocks in JSON to match BlockHistorySize")

	// Extract slots and compute unit prices from the blocks
	// We'll consider the last 'BlockHistorySize' blocks
	testSlots := make([]uint64, 0, len(testBlocks))
	testPrices := make([]ComputeUnitPrice, 0, len(testBlocks))
	startIndex := len(testBlocks) - int(depth)
	testBlocks = testBlocks[startIndex:]
	for _, block := range testBlocks {
		// extract compute unit prices and get median from each block
		slot := block.ParentSlot + 1
		testSlots = append(testSlots, slot)
		feeData, err := ParseBlock(block)
		require.NoError(t, err, "Failed to parse block at slot %d", slot)
		require.NotEmpty(t, feeData.Prices, "No compute unit prices found in block at slot %d", slot)
		medianPrice, err := mathutil.Median(feeData.Prices...)
		require.NoError(t, err, "Failed to calculate median price for block at slot %d", slot)
		testPrices = append(testPrices, medianPrice)
	}
	testSlotsResult := rpc.BlocksResult(testSlots)
	// Get avg of medians of each block
	multipleBlocksAvg, _ := mathutil.Avg(testPrices...)

	rw := clientmock.NewReaderWriter(t)
	rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
	rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
	rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
		Return(&testSlotsResult, nil)
	for i, slot := range testSlots {
		rw.On("GetBlock", mock.Anything, slot).
			Return(testBlocks[i], nil)
	}

	t.Run("Successful Estimation", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice).Maybe()
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, depth, pollPeriod)
		// Calculated avg price should be equal to the one extracted manually from the blocks.
		require.NoError(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth))
		assert.Equal(t, uint64(multipleBlocksAvg), estimator.BaseComputeUnitPrice())
		assert.Equal(t, float64(multipleBlocksAvg), testutil.ToFloat64(promBHEComputeUnitPrice.WithLabelValues(chainID)), "metric did not record compute unit price")
	})

	t.Run("Successful Estimation with partial cache fill", func(t *testing.T) {
		partialCacheRW := clientmock.NewReaderWriter(t)
		partialCacheRWLoader := func(ctx context.Context) (client.ReaderWriter, error) { return partialCacheRW, nil }
		partialCacheRW.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
		testSlotsResult := rpc.BlocksResult(testSlots[1:])
		partialCacheRW.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&testSlotsResult, nil)
		for i, slot := range testSlots {
			// Skip mocking the oldest block fetch because of partial load
			if i == 0 {
				continue
			}
			partialCacheRW.On("GetBlock", mock.Anything, slot).Return(testBlocks[i], nil).Once()
		}

		// Setup
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice).Maybe()
		cfg.On("BlockHistoryBatchLoadSize").Return(uint64(len(testBlocks) - 1)) // Set cache load batch smaller than depth to simulate partial cache
		estimator := initializeEstimator(ctx, t, partialCacheRWLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, depth-1, pollPeriod)

		// Calculated avg price should be equal to the one extracted manually from the blocks.
		require.NoError(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth))
		partialCacheAvg := 30250
		// Avg of block medians for the 2 latest blocks only due to partial cache fill
		assert.Equal(t, uint64(partialCacheAvg), estimator.BaseComputeUnitPrice())
		assert.Equal(t, float64(partialCacheAvg), testutil.ToFloat64(promBHEComputeUnitPrice.WithLabelValues(chainID)), "metric did not record compute unit price")
	})

	t.Run("Min Gate: Price Should Be Floored at Min", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMin := uint64(multipleBlocksAvg) + 100 // Set min higher than the avg price
		setupConfigMock(cfg, defaultPrice, tmpMin, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMin").Return(tmpMin)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, depth, pollPeriod)
		// Compute unit price should be floored at min
		require.NoError(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), "Failed to calculate price with price below min")
		assert.Equal(t, tmpMin, estimator.BaseComputeUnitPrice(), "Price should be floored at min")
	})

	t.Run("Max Gate: Price Should Be Capped at Max", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMax := uint64(multipleBlocksAvg) - 100 // Set tmpMax lower than the avg price
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(tmpMax)
		cfg.On("ComputeUnitPriceMin").Return(minPrice)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, depth, pollPeriod)

		// Compute unit price should be capped at max
		require.NoError(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), "Failed to calculate price with price above max")
		assert.Equal(t, tmpMax, estimator.BaseComputeUnitPrice(), "Price should be capped at max")
	})

	// Error handling scenarios
	t.Run("failed to get client", func(t *testing.T) {
		// Setup
		rwFailLoader := func(context.Context) (client.ReaderWriter, error) {
			// Return error to simulate failure to get client
			return nil, fmt.Errorf("fail client load")
		}
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		estimator := initializeEstimator(ctx, t, rwFailLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, 0, pollPeriod)

		// Price should remain unchanged
		require.Error(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), "Expected error when getting client fails")
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("failed to get current slot", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		rw.On("SlotHeight", mock.Anything).Return(uint64(0), fmt.Errorf("failed to get current slot")) // Mock SlotHeight returning error
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)
		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, 0, pollPeriod)
		// wait for populate cache to run
		time.Sleep(time.Millisecond * 50)
		// Price should remain unchanged
		require.Error(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), "Expected error when getting current slot fails")
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("current slot is less than desired block count", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		rw.On("SlotHeight", mock.Anything).Return(depth-1, nil) // Mock SlotHeight returning less than desiredBlockCount
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, 0, pollPeriod)
		// wait for populate cache to run
		time.Sleep(time.Millisecond * 50)
		// Price should remain unchanged
		require.Error(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), "Expected error when current slot is less than desired block count")
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("failed to get blocks with limit", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("failed to get blocks with limit")) // Mock GetBlocksWithLimit returning error
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, 0, pollPeriod)

		// wait for populate cache to run
		time.Sleep(time.Millisecond * 50)
		// Price should remain unchanged
		require.Error(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), "Expected error when getting blocks with limit fails")
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("no compute unit prices collected", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := func(ctx context.Context) (client.ReaderWriter, error) { return rw, nil }
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, depth)
		cfg.On("ComputeUnitPriceMax").Return(maxPrice)
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
		emptyBlocks := rpc.BlocksResult{} // No blocks with compute unit prices
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&emptyBlocks, nil)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to populate the cache and calculate the latest price
		waitForEstimation(t, estimator, 0, pollPeriod)

		// wait for populate cache to run
		time.Sleep(time.Millisecond * 50)
		// Price should remain unchanged
		require.EqualError(t, estimator.calculatePriceFromMultipleBlocks(t.Context(), depth), errNoComputeUnitPriceCollected.Error(), "Expected error when no compute unit prices are collected")
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("Cache successfully cleared of excess blocks", func(t *testing.T) {
		block1Slot := testSlots[0]
		block2Slot := testSlots[1]
		block3Slot := testSlots[2]
		olderEstimate := uint64(58750) // median price of oldest 2 blocks
		newerEstimate := uint64(30250) // median price of latest 2 blocks
		cleanCacheRw := clientmock.NewReaderWriter(t)
		cleanCacheRWLoader := func(ctx context.Context) (client.ReaderWriter, error) { return cleanCacheRw, nil }
		// Return second to last block as highest block
		cleanCacheRw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-2], nil).Once()
		// Return the oldest 2 blocks when get blocks is first called
		testSlotsResult := rpc.BlocksResult(testSlots[:2])
		cleanCacheRw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&testSlotsResult, nil).Once()
		for i, slot := range testSlots {
			cleanCacheRw.On("GetBlock", mock.Anything, slot).Return(testBlocks[i], nil).Once()
		}

		// Setup
		cfg := cfgmock.NewConfig(t)
		smallerDepth := len(testSlots) - 1
		setupConfigMock(cfg, defaultPrice, minPrice, pollPeriod, uint64(smallerDepth))
		cfg.On("ComputeUnitPriceMax").Return(maxPrice).Maybe()
		estimator := initializeEstimator(ctx, t, cleanCacheRWLoader, cfg, logger.Test(t), chainID)

		// Wait for estimator to estimate price on the older 2 blocks
		require.Eventually(t, func() bool {
			return estimator.BaseComputeUnitPrice() == olderEstimate
		}, 2*pollPeriod, 1*time.Second)

		estimator.cacheMu.RLock()
		require.Len(t, estimator.cache.storedBlockRange, smallerDepth)
		require.Len(t, estimator.cache.medianMap, smallerDepth)
		require.Contains(t, estimator.cache.medianMap, block1Slot)
		require.Contains(t, estimator.cache.medianMap, block2Slot)
		estimator.cacheMu.RUnlock()

		// Return second to last block as highest block
		cleanCacheRw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
		// Return the latest 2 blocks when get blocks is called again
		testSlotsResult = rpc.BlocksResult(testSlots[1:])
		cleanCacheRw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&testSlotsResult, nil)

		// Wait for estimator to estimate price on the latest 2 blocks
		require.Eventually(t, func() bool {
			return estimator.BaseComputeUnitPrice() == newerEstimate
		}, 2*pollPeriod, 1*time.Second)
		estimator.cacheMu.RLock()
		require.Len(t, estimator.cache.storedBlockRange, smallerDepth)
		require.Len(t, estimator.cache.medianMap, smallerDepth)
		require.Contains(t, estimator.cache.medianMap, block2Slot)
		require.Contains(t, estimator.cache.medianMap, block3Slot)
		estimator.cacheMu.RUnlock()
	})
}

// setupConfigMock configures the Config mock with necessary return values.
func setupConfigMock(cfg *cfgmock.Config, defaultPrice uint64, minPrice uint64, pollPeriod time.Duration, depth uint64) {
	cfg.On("ComputeUnitPriceDefault").Return(defaultPrice).Once()
	cfg.On("ComputeUnitPriceMin").Return(minPrice).Maybe()
	cfg.On("BlockHistoryPollPeriod").Return(pollPeriod).Once()
	cfg.On("BlockHistorySize").Return(depth)
	cfg.On("BlockHistoryBatchLoadSize").Return(uint64(20)).Maybe()
}

// initializeEstimator initializes, starts, and ensures cleanup of the BlockHistoryEstimator.
func initializeEstimator(ctx context.Context, t *testing.T, rwLoader func(context.Context) (client.ReaderWriter, error), cfg *cfgmock.Config, lgr logger.Logger, chainID string) *blockHistoryEstimator {
	estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, chainID)
	require.NoError(t, err, "Failed to create BlockHistoryEstimator")
	require.NoError(t, estimator.Start(ctx), "Failed to start BlockHistoryEstimator")

	// Ensure estimator is closed after the test
	t.Cleanup(func() {
		require.NoError(t, estimator.Close(), "Failed to close BlockHistoryEstimator")
	})

	return estimator
}

func readMultipleBlocksFromFile(t *testing.T, filePath string) []*rpc.GetBlockResult {
	// Read multiple blocks from JSON file
	testBlocksData, err := os.ReadFile(filePath)
	require.NoError(t, err)
	var testBlocks []*rpc.GetBlockResult
	require.NoError(t, json.Unmarshal(testBlocksData, &testBlocks))
	return testBlocks
}

func waitForEstimation(t *testing.T, estimator *blockHistoryEstimator, cacheSize uint64, pollPeriod time.Duration) {
	// Wait for estimator to populate the cache and calculate the latest price
	require.Eventually(t, func() bool {
		estimator.cacheMu.RLock()
		n := uint64(len(estimator.cache.medianMap))
		estimator.cacheMu.RUnlock()
		return estimator.BaseComputeUnitPrice() != 0 && n >= cacheSize
	}, 2*pollPeriod, 1*time.Second)
}
