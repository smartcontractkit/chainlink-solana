package fees

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	cfgmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
)

func TestBlockHistoryEstimator_LatestBlock(t *testing.T) {
	min := uint64(10)
	max := uint64(1000)

	rw := clientmock.NewReaderWriter(t)
	rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
		return rw, nil
	})
	cfg := cfgmock.NewConfig(t)
	cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
	cfg.On("ComputeUnitPriceMin").Return(min)
	cfg.On("ComputeUnitPriceMax").Return(max)
	cfg.On("BlockHistoryPollPeriod").Return(100 * time.Millisecond)
	lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
	ctx := tests.Context(t)

	// file contains legacy + v0 transactions
	testBlockData, err := os.ReadFile("./blockdata.json")
	require.NoError(t, err)
	blockRes := &rpc.GetBlockResult{}
	require.NoError(t, json.Unmarshal(testBlockData, blockRes))

	// happy path
	estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, LatestBlockEstimator)
	require.NoError(t, err)

	rw.On("GetLatestBlock", mock.Anything).Return(blockRes, nil).Once()
	require.NoError(t, estimator.Start(ctx))
	tests.AssertLogEventually(t, logs, "BlockHistoryEstimator: updated")
	assert.Equal(t, uint64(55000), estimator.readRawPrice())

	// min/max gates
	assert.Equal(t, max, estimator.BaseComputeUnitPrice())
	estimator.price = 0
	assert.Equal(t, min, estimator.BaseComputeUnitPrice())
	validPrice := uint64(100)
	estimator.price = validPrice
	assert.Equal(t, estimator.readRawPrice(), estimator.BaseComputeUnitPrice())

	// failed to get latest block
	rw.On("GetLatestBlock", mock.Anything).Return(nil, fmt.Errorf("fail rpc call")).Once()
	tests.AssertLogEventually(t, logs, "failed to get block")
	assert.Equal(t, validPrice, estimator.BaseComputeUnitPrice(), "price should not change when getPrice fails")

	// failed to parse block
	rw.On("GetLatestBlock", mock.Anything).Return(nil, nil).Once()
	tests.AssertLogEventually(t, logs, "failed to parse block")
	assert.Equal(t, validPrice, estimator.BaseComputeUnitPrice(), "price should not change when getPrice fails")

	// failed to calculate median
	rw.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Once()
	tests.AssertLogEventually(t, logs, "failed to find median")
	assert.Equal(t, validPrice, estimator.BaseComputeUnitPrice(), "price should not change when getPrice fails")

	// back to happy path
	rw.On("GetLatestBlock", mock.Anything).Return(blockRes, nil).Once()
	tests.AssertEventually(t, func() bool {
		return logs.FilterMessageSnippet("BlockHistoryEstimator: updated").Len() == 2
	})
	assert.Equal(t, uint64(55000), estimator.readRawPrice())
	require.NoError(t, estimator.Close())

	// failed to get client
	rwFail := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
		return nil, fmt.Errorf("fail client load")
	})
	estimator, err = NewBlockHistoryEstimator(rwFail, cfg, lgr, LatestBlockEstimator)
	require.NoError(t, err)
	require.NoError(t, estimator.Start(ctx))
	tests.AssertLogEventually(t, logs, "failed to get client")
	require.NoError(t, estimator.Close())
}

func TestBlockHistoryEstimator_MultipleBlocks(t *testing.T) {
	min := uint64(100)
	max := uint64(100_000)
	blockHistoryDepth := uint64(6)

	// Read multiple blocks from JSON file
	testBlocksData, err := os.ReadFile("./multiple_blocks_data.json")
	require.NoError(t, err)
	var testBlocks []*rpc.GetBlockResult
	require.NoError(t, json.Unmarshal(testBlocksData, &testBlocks))
	require.GreaterOrEqual(t, len(testBlocks), int(blockHistoryDepth), "Not enough blocks in JSON to match blockHistoryDepth")

	// Extract slots and compute unit prices from the blocks
	// We'll consider the last 'blockHistoryDepth' blocks
	var testSlots []uint64
	var testPrices []ComputeUnitPrice
	startIndex := len(testBlocks) - int(blockHistoryDepth)
	testBlocks = testBlocks[startIndex:]
	for _, block := range testBlocks {
		// extract compute unit prices and get median from each block
		slot := block.ParentSlot + 1
		testSlots = append(testSlots, slot)
		feeData, err := ParseBlock(block)
		require.NoError(t, err, "Failed to parse block at slot %d", slot)
		require.NotEmpty(t, feeData.Prices, "No compute unit prices found in block at slot %d", slot)
		medianPrice, err := mathutil.Median(feeData.Prices...)
		testPrices = append(testPrices, medianPrice)
	}

	testSlotsResult := rpc.BlocksResult(testSlots)

	t.Run("Successful MultipleBlocksEstimator", func(t *testing.T) {
		// Set up mocks for successful estimation
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(blockHistoryDepth)

		// Set up mock expectations
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil).Times(1)
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&testSlotsResult, nil).Times(1)
		for i, slot := range testSlots {
			rw.On("GetBlock", mock.Anything, slot).
				Return(testBlocks[i], nil).Times(1)
		}

		lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Start the estimator and wait for update
		estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))
		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()
		tests.AssertLogEventually(t, logs, "BlockHistoryEstimator: updated")

		// Calculate expected median price from all the blocks and check estimated price
		expectedMedianPrice, err := mathutil.Median(testPrices...)
		require.NoError(t, err)
		assert.Equal(t, uint64(expectedMedianPrice), estimator.BaseComputeUnitPrice())
		if uint64(expectedMedianPrice) > max {
			assert.Equal(t, max, estimator.BaseComputeUnitPrice())
		} else if uint64(expectedMedianPrice) < min {
			assert.Equal(t, min, estimator.BaseComputeUnitPrice())
		} else {
			assert.Equal(t, uint64(expectedMedianPrice), estimator.BaseComputeUnitPrice())
		}
	})

	t.Run("desiredBlockCount is zero", func(t *testing.T) {
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(uint64(0)) // desiredBlockCount is zero

		lgr, _ := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Start the estimator
		estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))

		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()

		// Since desiredBlockCount is zero, calculation should fail
		err = estimator.calculatePrice(ctx)
		require.Error(t, err)
		assert.Equal(t, "desiredBlockCount is zero", err.Error())
	})

	// Error handling scenarios
	t.Run("failed to get client", func(t *testing.T) {
		// Set up a failing client loader
		rwFailLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return nil, fmt.Errorf("fail client load")
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(blockHistoryDepth)

		lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Start the estimator with failing client
		estimator, err := NewBlockHistoryEstimator(rwFailLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))

		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()

		// Wait for the estimator to log the failure
		tests.AssertLogEventually(t, logs, "failed to get client")

		// Ensure the price remains unchanged (default)
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice())
	})

	t.Run("failed to get current slot", func(t *testing.T) {
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(blockHistoryDepth)

		// Mock failed SlotHeight
		rw.On("SlotHeight", mock.Anything).Return(uint64(0), fmt.Errorf("failed to get current slot")).Times(1)

		lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Start the estimator
		estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))

		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()

		// Wait for the estimator to log the failure
		tests.AssertLogEventually(t, logs, "failed to get current slot")

		// Ensure the price remains unchanged
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice())
	})

	t.Run("current slot is less than desired block count", func(t *testing.T) {
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(blockHistoryDepth)

		lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Mock SlotHeight returning 5, less than desiredBlockCount=6
		rw.On("SlotHeight", mock.Anything).Return(uint64(5), nil).Once()

		// Start the estimator
		estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))

		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()

		// Assert failure and ensure the price remains unchanged
		tests.AssertLogEventually(t, logs, "current slot is less than desired block count")
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice())
	})

	t.Run("failed to get blocks with limit", func(t *testing.T) {
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(blockHistoryDepth)

		// Mock successful SlotHeight
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil).Times(1)
		// Mock failed GetBlocksWithLimit
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("failed to get blocks with limit")).Times(1)

		lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Start the estimator
		estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))

		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()

		// Assert failure and ensure the price remains unchanged
		tests.AssertLogEventually(t, logs, "failed to get blocks with limit")
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice())
	})

	t.Run("no compute unit prices collected", func(t *testing.T) {
		rw := clientmock.NewReaderWriter(t)
		rwLoader := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
			return rw, nil
		})
		cfg := cfgmock.NewConfig(t)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("ComputeUnitPriceMin").Return(min)
		cfg.On("ComputeUnitPriceMax").Return(max)
		cfg.On("ComputeUnitPriceDefault").Return(uint64(100))
		cfg.On("BlockHistoryPollPeriod").Return(3 * time.Second)
		cfg.On("BlockHistoryDepth").Return(blockHistoryDepth)

		// Mock SlotHeight
		rw.On("SlotHeight", mock.Anything).Return(testSlots[len(testSlots)-1], nil).Once()
		// Mock GetBlocksWithLimit returning empty result
		emptyBlocks := rpc.BlocksResult{}
		rw.On("GetBlocksWithLimit", mock.Anything, mock.Anything, mock.Anything).
			Return(&emptyBlocks, nil).Once()

		lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
		ctx := tests.Context(t)

		// Start the estimator
		estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr, MultipleBlocksEstimator)
		require.NoError(t, err)
		require.NoError(t, estimator.Start(ctx))

		// Ensure estimator is closed after the test
		defer func() {
			require.NoError(t, estimator.Close())
		}()

		// Assert failure and ensure the price remains unchanged
		tests.AssertLogEventually(t, logs, "no compute unit prices collected")
		assert.Equal(t, uint64(100), estimator.BaseComputeUnitPrice())
	})
}
