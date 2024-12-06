package fees

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	cfgmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
)

func TestBlockHistoryEstimator_InvalidBlockHistorySize(t *testing.T) {
	// Setup
	invalidDepth := uint64(0) // Invalid value to trigger error
	rw := clientmock.NewReaderWriter(t)
	rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
		return rw, nil
	})
	cfg := cfgmock.NewConfig(t)
	cfg.On("BlockHistorySize").Return(invalidDepth)

	// Initialize estimator and expect an error due to invalid BlockHistorySize
	_, err := NewBlockHistoryEstimator(rwLoader, cfg, logger.Test(t))
	require.Error(t, err, "Expected error for invalid block history size")
	assert.Equal(t, "invalid block history depth: 0", err.Error(), "Unexpected error message for invalid block history size")
}

func TestBlockHistoryEstimator_LatestBlock(t *testing.T) {
	// Helper variables for tests
	min := uint64(10)
	max := uint64(100_000)
	defaultPrice := uint64(100)
	depth := uint64(1) // 1 is LatestBlockEstimator
	pollPeriod := 100 * time.Millisecond
	ctx := tests.Context(t)

	// Grabbing last block of multiple_blocks file to use as latest block
	testBlocks := readMultipleBlocksFromFile(t, "./multiple_blocks_data.json")
	lastBlock := testBlocks[len(testBlocks)-1]
	lastBlockFeeData, _ := ParseBlock(lastBlock)
	lastBlockMedianPrice, _ := mathutil.Median(lastBlockFeeData.Prices...)

	rw := clientmock.NewReaderWriter(t)
	rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
		return rw, nil
	})
	rw.On("GetLatestBlock", mock.Anything).Return(lastBlock, nil)

	t.Run("Successful Estimation", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Assert the computed price matches the expected price
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price")
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, uint64(lastBlockMedianPrice), estimator.BaseComputeUnitPrice())
	})

	t.Run("Min Gate: Price Should Be Floored at Min", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMin := uint64(lastBlockMedianPrice) + 100 // Set min higher than the median price
		setupConfigMock(cfg, defaultPrice, tmpMin, max, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

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
		setupConfigMock(cfg, defaultPrice, min, tmpMax, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Call calculatePrice and ensure no error
		// Assert the compute unit price is capped at max
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price with price above max")
		cfg.On("ComputeUnitPriceMax").Return(tmpMax)
		cfg.On("ComputeUnitPriceMin").Return(min)
		assert.Equal(t, tmpMax, estimator.BaseComputeUnitPrice(), "Price should be capped at max")
	})

	t.Run("Failed to Get Latest Block", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("GetLatestBlock", mock.Anything).Return(nil, fmt.Errorf("fail rpc call")) // Mock GetLatestBlock returning error
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Ensure the price remains unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when GetLatestBlock fails")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice(), "Price should not change when GetLatestBlock fails")
	})

	t.Run("Failed to Parse Block", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("GetLatestBlock", mock.Anything).Return(nil, nil) // Mock GetLatestBlock returning nil
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Ensure the price remains unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when parsing fails")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice(), "Price should not change when parsing fails")
	})

	t.Run("no compute unit prices collected", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil) // Mock GetLatestBlock returning empty block
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Ensure the price remains unchanged
		require.EqualError(t, estimator.calculatePrice(ctx), errNoComputeUnitPriceCollected.Error(), "Expected error when no compute unit prices are collected")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice(), "Price should not change when median calculation fails")
	})

	t.Run("Failed to Get Client", func(t *testing.T) {
		// Setup
		rwFailLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			// Return error to simulate failure to get client
			return nil, fmt.Errorf("fail client load")
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwFailLoader, cfg, logger.Test(t))

		// Call calculatePrice and expect an error
		// Ensure the price remains unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when getting client fails")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice(), "Price should remain at default when client fails")
	})
}

func TestBlockHistoryEstimator_MultipleBlocks(t *testing.T) {
	// helpers vars for tests
	min := uint64(100)
	max := uint64(100_000)
	depth := uint64(3)
	defaultPrice := uint64(100)
	pollPeriod := 3 * time.Second
	ctx := tests.Context(t)

	// Read multiple blocks from JSON file
	testBlocks := readMultipleBlocksFromFile(t, "./multiple_blocks_data.json")
	require.GreaterOrEqual(t, len(testBlocks), int(depth), "Not enough blocks in JSON to match BlockHistorySize")

	// Extract slots and compute unit prices from the blocks
	// We'll consider the last 'BlockHistorySize' blocks
	var testSlots []uint64
	var testPrices []ComputeUnitPrice
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
	rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
		return rw, nil
	})
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
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Calculated avg price should be equal to the one extracted manually from the blocks.
		require.NoError(t, estimator.calculatePrice(ctx))
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, uint64(multipleBlocksAvg), estimator.BaseComputeUnitPrice())
	})

	t.Run("Min Gate: Price Should Be Floored at Min", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMin := uint64(multipleBlocksAvg) + 100 // Set min higher than the avg price
		setupConfigMock(cfg, defaultPrice, tmpMin, max, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Compute unit price should be floored at min
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price with price below min")
		cfg.On("ComputeUnitPriceMin").Return(tmpMin)
		assert.Equal(t, tmpMin, estimator.BaseComputeUnitPrice(), "Price should be floored at min")
	})

	t.Run("Max Gate: Price Should Be Capped at Max", func(t *testing.T) {
		// Setup
		cfg := cfgmock.NewConfig(t)
		tmpMax := uint64(multipleBlocksAvg) - 100 // Set tmpMax lower than the avg price
		setupConfigMock(cfg, defaultPrice, min, tmpMax, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Compute unit price should be capped at max
		require.NoError(t, estimator.calculatePrice(ctx), "Failed to calculate price with price above max")
		cfg.On("ComputeUnitPriceMax").Return(tmpMax)
		cfg.On("ComputeUnitPriceMin").Return(min)
		assert.Equal(t, tmpMax, estimator.BaseComputeUnitPrice(), "Price should be capped at max")
	})

	// Error handling scenarios
	t.Run("failed to get client", func(t *testing.T) {
		// Setup
		rwFailLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			// Return error to simulate failure to get client
			return nil, fmt.Errorf("fail client load")
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		estimator := initializeEstimator(ctx, t, rwFailLoader, cfg, logger.Test(t))

		// Price should remain unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when getting client fails")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("failed to get current slot", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("SlotHeight", mock.Anything).Return(uint64(0), fmt.Errorf("failed to get current slot")) // Mock SlotHeight returning error
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Price should remain unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when getting current slot fails")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("current slot is less than desired block count", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("SlotHeight", mock.Anything).Return(depth-1, nil) // Mock SlotHeight returning less than desiredBlockCount
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Price should remain unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when current slot is less than desired block count")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("failed to get blocks with limit", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("failed to get blocks with limit")) // Mock GetBlocksWithLimit returning error
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Price should remain unchanged
		require.Error(t, estimator.calculatePrice(ctx), "Expected error when getting blocks with limit fails")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})

	t.Run("no compute unit prices collected", func(t *testing.T) {
		// Setup
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		setupConfigMock(cfg, defaultPrice, min, max, pollPeriod, depth)
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil)
		emptyBlocks := rpc.BlocksResult{} // No blocks with compute unit prices
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&emptyBlocks, nil)
		estimator := initializeEstimator(ctx, t, rwLoader, cfg, logger.Test(t))

		// Price should remain unchanged
		require.EqualError(t, estimator.calculatePrice(ctx), errNoComputeUnitPriceCollected.Error(), "Expected error when no compute unit prices are collected")
		cfg.On("ComputeUnitPriceMax").Return(max)
		assert.Equal(t, defaultPrice, estimator.BaseComputeUnitPrice())
	})
}

// setupConfigMock configures the Config mock with necessary return values.
func setupConfigMock(cfg *cfgmock.Config, defaultPrice uint64, min, max uint64, pollPeriod time.Duration, depth uint64) {
	cfg.On("ComputeUnitPriceDefault").Return(defaultPrice).Once()
	cfg.On("ComputeUnitPriceMin").Return(min).Once()
	cfg.On("BlockHistoryPollPeriod").Return(pollPeriod).Once()
	cfg.On("BlockHistorySize").Return(depth)
}

// initializeEstimator initializes, starts, and ensures cleanup of the BlockHistoryEstimator.
func initializeEstimator(ctx context.Context, t *testing.T, rwLoader *utils.LazyLoad[client.ReaderWriter], cfg *cfgmock.Config, lgr logger.Logger) *blockHistoryEstimator {
	estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr)
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
