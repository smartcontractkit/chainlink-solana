package fees

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	cfgmock "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
)

func TestBlockHistoryEstimator(t *testing.T) {
	feePolling = 100 * time.Millisecond // TODO: make this part of cfg mock
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
	lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
	ctx := tests.Context(t)

	// file contains legacy + v0 transactions
	testBlockData, err := ioutil.ReadFile("./blockdata.json")
	require.NoError(t, err)
	blockRes := &rpc.GetBlockResult{}
	require.NoError(t, json.Unmarshal(testBlockData, blockRes))

	// happy path
	estimator, err := NewBlockHistoryEstimator(rwLoader, cfg, lgr)
	require.NoError(t, err)

	rw.On("GetLatestBlock").Return(blockRes, nil).Once()
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
	rw.On("GetLatestBlock").Return(nil, fmt.Errorf("fail rpc call")).Once()
	tests.AssertLogEventually(t, logs, "failed to get block")
	assert.Equal(t, validPrice, estimator.BaseComputeUnitPrice(), "price should not change when getPrice fails")

	// failed to parse block
	rw.On("GetLatestBlock").Return(nil, nil).Once()
	tests.AssertLogEventually(t, logs, "failed to parse block")
	assert.Equal(t, validPrice, estimator.BaseComputeUnitPrice(), "price should not change when getPrice fails")

	// failed to calculate median
	rw.On("GetLatestBlock").Return(&rpc.GetBlockResult{}, nil).Once()
	tests.AssertLogEventually(t, logs, "failed to find median")
	assert.Equal(t, validPrice, estimator.BaseComputeUnitPrice(), "price should not change when getPrice fails")

	// back to happy path
	rw.On("GetLatestBlock").Return(blockRes, nil).Once()
	tests.AssertEventually(t, func() bool {
		return logs.FilterMessageSnippet("BlockHistoryEstimator: updated").Len() == 2
	})
	assert.Equal(t, uint64(55000), estimator.readRawPrice())
	require.NoError(t, estimator.Close())

	// failed to get client
	rwFail := utils.NewLazyLoad(func() (client.ReaderWriter, error) {
		return nil, fmt.Errorf("fail client load")
	})
	estimator, err = NewBlockHistoryEstimator(rwFail, cfg, lgr)
	require.NoError(t, err)
	require.NoError(t, estimator.Start(ctx))
	tests.AssertLogEventually(t, logs, "failed to get client")
	require.NoError(t, estimator.Close())
}
