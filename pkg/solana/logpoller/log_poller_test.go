package logpoller

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
)

type mockedLP struct {
	ORM       *MockORM
	Client    *mocks.RPCClient
	Loader    *mockLogsLoader
	Filters   *mockFilters
	LogPoller *LogPoller
}

func newMockedLP(t *testing.T) mockedLP {
	result := mockedLP{
		ORM:     NewMockORM(t),
		Client:  mocks.NewRPCClient(t),
		Loader:  newMockLogsLoader(t),
		Filters: newMockFilters(t),
	}
	result.LogPoller = New(logger.TestSugared(t), result.ORM, result.Client)
	result.LogPoller.loader = result.Loader
	result.LogPoller.filters = result.Filters
	return result
}

func TestLogPoller_start(t *testing.T) {
	t.Run("Returns error if failed to load filters", func(t *testing.T) {
		lp := New(logger.TestSugared(t), nil, nil)
		filtersMock := newMockFilters(t)
		lp.filters = filtersMock
		expectedErr := errors.New("failed to load filters")
		filtersMock.EXPECT().LoadFilters(mock.Anything).Return(expectedErr).Once()
		err := lp.Start(tests.Context(t))
		require.ErrorIs(t, err, expectedErr)
	})
}

func TestLogPoller_run(t *testing.T) {
	t.Run("Aborts backfill if loader fails", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return([]Filter{{StartingBlock: 16}}).Once()
		expectedErr := errors.New("loaderFailed")
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, uint64(16), uint64(128)).Return(nil, nil, expectedErr).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("Backfill happy path", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return([]Filter{
			{ID: 1, StartingBlock: 16, Address: PublicKey{1, 2, 3}},
			{ID: 2, StartingBlock: 12, Address: PublicKey{1, 2, 3}},
			{ID: 3, StartingBlock: 14, Address: PublicKey{3, 2, 1}},
		}).Once()
		done := func() {}
		blocks := make(chan Block)
		close(blocks)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, []PublicKey{{1, 2, 3}, {3, 2, 1}}, uint64(12), uint64(128)).Return(blocks, done, nil).Once()
		lp.Filters.EXPECT().MarkFilterBackfilled(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, filterID int64) error {
			switch filterID {
			case 1:
				return errors.New("filter no longer exists")
			case 2, 3:
				return nil
			default:
				require.Fail(t, "unexpected filter ID")
				return nil
			}
		}).Times(3)
		err := lp.LogPoller.run(tests.Context(t))
		require.ErrorContains(t, err, "failed to mark filter 1 backfilled: filter no longer exists")
	})
	t.Run("Returns error, if failed to get address for global backfill", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return(nil).Once()
		expectedErr := errors.New("failed to load filters")
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return(nil, expectedErr).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.ErrorContains(t, err, "failed getting addresses: failed to load filters")
	})
	t.Run("Aborts if there is no addresses", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return(nil).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return(nil, nil).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.NoError(t, err)
	})
	t.Run("Returns error, if failed to get latest slot", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return(nil).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]PublicKey{{}}, nil).Once()
		expectedErr := errors.New("RPC failed")
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(0, expectedErr).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("Returns error, if last processed slot is higher than latest finalized", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return(nil).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]PublicKey{{}}, nil).Once()
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(16, nil).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.ErrorContains(t, err, "last processed slot 128 is higher than highest RPC slot 16")
	})
	t.Run("Returns error, if fails to do block backfill", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return(nil).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]PublicKey{{}}, nil).Once()
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(130, nil).Once()
		expectedError := errors.New("failed to start backfill")
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, uint64(129), uint64(130)).Return(nil, nil, expectedError).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.ErrorContains(t, err, "failed processing block range [129, 130]: error backfilling filters: failed to start backfill")
	})
	t.Run("Happy path", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().GetFiltersToBackfill().Return(nil).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]PublicKey{{}}, nil).Once()
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(130, nil).Once()
		blocks := make(chan Block)
		close(blocks)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, uint64(129), uint64(130)).Return(blocks, func() {}, nil).Once()
		err := lp.LogPoller.run(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, int64(130), lp.LogPoller.lastProcessedSlot)
	})
}

func TestLogPoller_getLastProcessedSlot(t *testing.T) {
	t.Run("Returns cached value if available", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 10
		result, err := lp.LogPoller.getLastProcessedSlot(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, int64(10), result)
	})
	t.Run("Returns error if failed to read from db", func(t *testing.T) {
		lp := newMockedLP(t)
		expectedErr := errors.New("failed to read from db")
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(0, expectedErr).Once()
		_, err := lp.LogPoller.getLastProcessedSlot(tests.Context(t))
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("Reads latest processed from db", func(t *testing.T) {
		lp := newMockedLP(t)
		expectedValue := int64(10)
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(expectedValue, nil).Once()
		result, err := lp.LogPoller.getLastProcessedSlot(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, expectedValue, result)
	})
	t.Run("Returns error if failed to read from DB (no data) and RPC", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(0, sql.ErrNoRows).Once()
		expectedError := errors.New("RPC failed")
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(0, expectedError).Once()
		_, err := lp.LogPoller.getLastProcessedSlot(tests.Context(t))
		require.ErrorIs(t, err, expectedError)
	})
	t.Run("Returns error if genesis block is the latest finalized", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(0, sql.ErrNoRows).Once()
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(0, nil).Once()
		_, err := lp.LogPoller.getLastProcessedSlot(tests.Context(t))
		require.ErrorContains(t, err, "latest finalized slot is 0 - waiting for next slot to start processing")
	})
	t.Run("Returns block before latest finalized as last processed if using RPC", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(0, sql.ErrNoRows).Once()
		const latestFinalized = uint64(10)
		lp.Client.EXPECT().GetSlot(mock.Anything, rpc.CommitmentFinalized).Return(latestFinalized, nil).Once()
		actual, err := lp.LogPoller.getLastProcessedSlot(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, int64(latestFinalized-1), actual)
	})
}

func TestLogPoller_processBlocksRange(t *testing.T) {
	t.Run("Returns error if failed to start backfill", func(t *testing.T) {
		lp := newMockedLP(t)
		expectedErr := errors.New("failed to start backfill")
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, expectedErr).Once()
		err := lp.LogPoller.processBlocksRange(tests.Context(t), nil, 10, 20)
		require.ErrorIs(t, err, expectedErr)
	})
	funcWithCallExpectation := func(t *testing.T) func() {
		var called atomic.Bool
		t.Cleanup(func() {
			require.True(t, called.Load(), "expected function to be called")
		})
		return func() { called.Store(true) }
	}
	t.Run("Can abort by cancelling context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(tests.Context(t))
		lp := newMockedLP(t)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(context.Context, []PublicKey, uint64, uint64) (<-chan Block, func(), error) {
			cancel()
			return nil, funcWithCallExpectation(t), nil
		}).Once()
		err := lp.LogPoller.processBlocksRange(ctx, nil, 10, 20)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("Happy path", func(t *testing.T) {
		lp := newMockedLP(t)
		blocks := make(chan Block, 2)
		blocks <- Block{}
		blocks <- Block{}
		close(blocks)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(blocks, funcWithCallExpectation(t), nil).Once()
		err := lp.LogPoller.processBlocksRange(tests.Context(t), nil, 10, 20)
		require.NoError(t, err)
	})
}
