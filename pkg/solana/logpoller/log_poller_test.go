package logpoller

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncfg "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type mockedLP struct {
	ORM       *mocks.MockORM
	Client    *mocks.RPCClient
	Loader    *mocks.MockLogsLoader
	Filters   *mocks.MockFilters
	LogPoller *Service
}

func newMockedLPwithConfig(t *testing.T, cfg config.Config, chainID string) mockedLP {
	result := mockedLP{
		ORM:     mocks.NewMockORM(t),
		Client:  mocks.NewRPCClient(t),
		Loader:  mocks.NewMockLogsLoader(t),
		Filters: mocks.NewMockFilters(t),
	}

	var err error
	result.LogPoller, err = New(logger.TestSugared(t), result.ORM, result.Client, cfg, chainID)
	require.NoError(t, err)
	result.LogPoller.loader = result.Loader
	result.LogPoller.filters = result.Filters
	return result
}

func newMockedLP(t *testing.T) mockedLP {
	cfg := config.NewDefault()
	cfg.Chain.LogPollerSlotsBatchSize = ptr[int64](100)
	return newMockedLPwithConfig(t, cfg, chainID)
}

func TestLogPoller_run(t *testing.T) {
	t.Run("Abort run if failed to load filters", func(t *testing.T) {
		lp := newMockedLP(t)
		expectedErr := errors.New("failed to load filters")
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(expectedErr).Once()
		err := lp.LogPoller.run(t.Context())
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("Aborts backfill if loader fails", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return([]types.Filter{{StartingBlock: 16}}, 16).Once()

		expectedErr := errors.New("loaderFailed")
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, uint64(16), uint64(128)).Return(nil, nil, expectedErr).Once()
		err := lp.LogPoller.run(t.Context())
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("Backfill happy path", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return([]types.Filter{
			{ID: 1, StartingBlock: 16, Address: types.PublicKey{1, 2, 3}},
			{ID: 2, StartingBlock: 12, Address: types.PublicKey{1, 2, 3}},
			{ID: 3, StartingBlock: 14, Address: types.PublicKey{3, 2, 1}},
		}, 12).Once()
		done := func() {}
		blocks := make(chan types.Block)
		close(blocks)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, []types.PublicKey{{1, 2, 3}, {3, 2, 1}}, uint64(12), uint64(128)).Return(blocks, done, nil).Once()
		lp.Filters.EXPECT().UpdateBackfillProgress(mock.Anything, mock.Anything, int64(128), true).RunAndReturn(func(ctx context.Context, filter types.Filter, _ int64, _ bool) error {
			switch filter.ID {
			case 1:
				return errors.New("filter no longer exists")
			case 2, 3:
				return nil
			default:
				require.Fail(t, "unexpected filter ID")
				return nil
			}
		}).Times(3)
		err := lp.LogPoller.run(t.Context())
		require.ErrorContains(t, err, "failed to mark filter 1 backfilled: filter no longer exists")
	})
	t.Run("Returns error, if failed to get address for global backfill", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		expectedErr := errors.New("failed to load filters")
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return(nil, expectedErr).Once()
		err := lp.LogPoller.run(t.Context())
		require.ErrorContains(t, err, "failed getting addresses: failed to load filters")
	})
	t.Run("Aborts if there is no addresses", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return(nil, nil).Once()
		err := lp.LogPoller.run(t.Context())
		require.NoError(t, err)
	})
	t.Run("Returns error, if failed to get latest slot", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]types.PublicKey{{}}, nil).Once()
		expectedErr := errors.New("RPC failed")
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(0, expectedErr).Once()
		err := lp.LogPoller.run(t.Context())
		require.ErrorIs(t, err, expectedErr)
	})
	t.Run("Returns error, if last processed slot is higher than latest finalized", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]types.PublicKey{{}}, nil).Once()
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(16, nil).Once()
		err := lp.LogPoller.run(t.Context())
		require.ErrorContains(t, err, "last processed slot 128 is higher than highest RPC slot 16")
	})
	t.Run("Returns error, if fails to do block backfill", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]types.PublicKey{{}}, nil).Once()
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(130, nil).Once()
		expectedError := errors.New("failed to start backfill")
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, uint64(129), uint64(130)).Return(nil, nil, expectedError).Once()
		err := lp.LogPoller.run(t.Context())
		require.ErrorContains(t, err, "error backfilling filters: failed to start backfill")
	})
	t.Run("Happy path", func(t *testing.T) {
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 128
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]types.PublicKey{{}}, nil).Once()
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(230, nil).Once()
		blocks := make(chan types.Block)
		close(blocks)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, uint64(129), uint64(230)).Return(blocks, func() {}, nil).Once()
		err := lp.LogPoller.run(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(230), lp.LogPoller.lastProcessedSlot)
	})
	// These two sub-tests demonstrate the difference between single-batch and multi-batch
	// failure scenarios, showing how the batch-level cursor update prevents redundant RPC calls.
	//
	// Both use the same range [101..105] with a failure at block 103. The difference is
	// whether blocks land in one batch or multiple batches before the failure.
	t.Run("All blocks in one batch: failure re-fetches entire range", func(t *testing.T) {
		// When all blocks arrive in a single batch and processing fails,
		// the cursor update never executes — lastProcessedSlot stays at its
		// original value, so the retry re-fetches everything from the start.
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 100
		addresses := []types.PublicKey{{1}}

		var backfillFromSlots []uint64

		lp.LogPoller.processBlocks = func(_ context.Context, _ []types.Block) error {
			return errors.New("simulated error processing batch")
		}

		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil)
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Twice()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return(addresses, nil)
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(uint64(105), nil)

		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, addresses, mock.AnythingOfType("uint64"), uint64(105)).
			RunAndReturn(func(_ context.Context, _ []types.PublicKey, from, _ uint64) (<-chan types.Block, func(), error) {
				backfillFromSlots = append(backfillFromSlots, from)
				ch := make(chan types.Block, 3)
				ch <- types.Block{SlotNumber: 101}
				ch <- types.Block{SlotNumber: 102}
				ch <- types.Block{SlotNumber: 103}
				close(ch)
				return ch, func() {}, nil
			})

		// --- Run 1: single batch [101,102,103] fails ---
		err := lp.LogPoller.run(t.Context())
		require.Error(t, err)
		assert.Equal(t, int64(100), lp.LogPoller.lastProcessedSlot,
			"cursor stays at 100 — batch failed before cursor could update")

		// --- Run 2: starts from 101 again, re-fetching all blocks ---
		err = lp.LogPoller.run(t.Context())
		require.Error(t, err)

		require.Len(t, backfillFromSlots, 2)
		assert.Equal(t, uint64(101), backfillFromSlots[0], "run 1: starts from 101")
		assert.Equal(t, uint64(101), backfillFromSlots[1],
			"run 2: starts from 101 AGAIN — all blocks re-fetched because no batch succeeded")
	})
	t.Run("Split batches: failure only re-fetches unprocessed blocks", func(t *testing.T) {
		// When blocks arrive in separate batches and the first batch succeeds,
		// the cursor advances. The retry only fetches blocks after the cursor.
		lp := newMockedLP(t)
		lp.LogPoller.lastProcessedSlot = 100
		addresses := []types.PublicKey{{1}}

		var backfillFromSlots []uint64

		callCount := 0
		blocks1 := make(chan types.Block, 2)
		blocks1 <- types.Block{SlotNumber: 101}
		blocks1 <- types.Block{SlotNumber: 102}

		lp.LogPoller.processBlocks = func(_ context.Context, batch []types.Block) error {
			callCount++
			if callCount == 1 {
				// First batch [101, 102] succeeds. Inject the failing block for the next batch.
				blocks1 <- types.Block{SlotNumber: 103}
				close(blocks1)
				return nil
			}
			if callCount == 2 {
				return errors.New("simulated error processing block 103")
			}
			return nil
		}

		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil)
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return(addresses, nil)
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(uint64(105), nil)

		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, addresses, mock.AnythingOfType("uint64"), uint64(105)).
			RunAndReturn(func(_ context.Context, _ []types.PublicKey, from, _ uint64) (<-chan types.Block, func(), error) {
				backfillFromSlots = append(backfillFromSlots, from)
				if from == 101 {
					return blocks1, func() {}, nil
				}
				ch := make(chan types.Block, 3)
				ch <- types.Block{SlotNumber: 103}
				ch <- types.Block{SlotNumber: 104}
				ch <- types.Block{SlotNumber: 105}
				close(ch)
				return ch, func() {}, nil
			})

		// --- Run 1: batch [101,102] succeeds, batch [103] fails ---
		err := lp.LogPoller.run(t.Context())
		require.Error(t, err)
		assert.Equal(t, int64(102), lp.LogPoller.lastProcessedSlot,
			"cursor advances to 102 — first batch succeeded before failure")

		// --- Run 2: starts from 103, blocks 101 and 102 are NOT re-fetched ---
		lp.Filters.EXPECT().GetFiltersToBackfill(lp.LogPoller.lastProcessedSlot).Return(nil, lp.LogPoller.lastProcessedSlot).Once()
		err = lp.LogPoller.run(t.Context())
		require.NoError(t, err)
		assert.Equal(t, int64(105), lp.LogPoller.lastProcessedSlot)

		require.Len(t, backfillFromSlots, 2)
		assert.Equal(t, uint64(101), backfillFromSlots[0], "run 1: starts from 101")
		assert.Equal(t, uint64(103), backfillFromSlots[1],
			"run 2: starts from 103 — blocks 101,102 are NOT re-fetched")
	})
	t.Run("Gracefully handles failure during backfill", func(t *testing.T) {
		// Ensure that failure to backfill a batch does not cause false advancement of the cursor for filters that require backfill.
		lp := newMockedLP(t)

		const dbHead int64 = 1000
		addr := types.PublicKey{1}
		filter := types.Filter{ID: 7, StartingBlock: 100, Address: addr}

		// First run only: lastProcessedSlot is 0, so we consult the DB and lookback RPCs.
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(dbHead, nil).Once()
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(uint64(1500), nil).Once()
		lp.Client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(0), nil).Once()

		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Twice()
		lp.Filters.EXPECT().GetFiltersToBackfill(dbHead).Return([]types.Filter{filter}, 100).Twice()

		var backfillRanges [][2]int64
		var processBlocksFailed bool
		lp.LogPoller.processBlocks = func(_ context.Context, batch []types.Block) error {
			for _, b := range batch {
				if b.SlotNumber > 200 && !processBlocksFailed {
					processBlocksFailed = true
					return errors.New("simulated failure after slot 200")
				}
			}
			return nil
		}

		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, []types.PublicKey{addr}, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ []types.PublicKey, from, to uint64) (<-chan types.Block, func(), error) {
				// nolint:gosec
				// G115: from and to are always within int64
				backfillRanges = append(backfillRanges, [2]int64{int64(from), int64(to)})
				// nolint:gosec
				// G115: to-from is always within int
				ch := make(chan types.Block, int(to-from)+2)
				go func() {
					defer close(ch)
					for s := from; s <= to; s++ {
						ch <- types.Block{SlotNumber: s}
					}
				}()
				return ch, func() {}, nil
			}).Twice()

		isFirstRun := true
		// Run 1: full intended range [100, dbHead]; fails once a batch contains a slot > 200.
		lp.Filters.EXPECT().UpdateBackfillProgress(mock.Anything, filter, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, filter types.Filter, lastProcessed int64, isComplete bool) error {
			if isFirstRun {
				require.Less(t, lastProcessed, int64(200), "cursor should not advance past the failing slot on first run")
				require.False(t, isComplete, "backfill should not be marked complete on first run")
			}
			return nil
		})
		err := lp.LogPoller.run(t.Context())
		require.Error(t, err)

		require.Len(t, backfillRanges, 1)
		assert.Equal(t, int64(100), backfillRanges[0][0], "first attempt should start at filter StartingBlock")
		assert.Equal(t, dbHead, backfillRanges[0][1], "first attempt should use persisted head as upper bound")

		isFirstRun = false
		err = lp.LogPoller.run(t.Context())
		require.NoError(t, err)

		require.Len(t, backfillRanges, 2)
		assert.Equal(t, int64(100), backfillRanges[1][0])
		assert.Equal(t, dbHead, backfillRanges[1][1])
	})
	t.Run("Newly inserted filter is properly backfilled", func(t *testing.T) {
		lp := newMockedLP(t)

		addr1 := types.PublicKey{1, 2, 3}
		addr2 := types.PublicKey{4, 5, 6}
		ctx := t.Context()

		// DB latest block 1000 via ORM only (do not set lastProcessedSlot on the service).
		lp.ORM.EXPECT().GetLatestBlock(mock.Anything).Return(int64(1000), nil).Once()
		lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(uint64(1001), nil).Once()
		lp.Client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(0), nil).Once()

		// Run 1: filter needs backfill from 100; loader only yields a block at 600 (due to sparse events).
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(int64(1000)).Return([]types.Filter{
			{ID: 1, StartingBlock: 100, Address: addr1},
		}, 100).Once()

		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, []types.PublicKey{addr1}, uint64(100), uint64(1000)).
			RunAndReturn(func(ctx context.Context, _ []types.PublicKey, _ uint64, _ uint64) (<-chan types.Block, func(), error) {
				blocks := make(chan types.Block, 4)
				blocks <- types.Block{SlotNumber: 101}
				blocks <- types.Block{SlotNumber: 102}
				blocks <- types.Block{SlotNumber: 103} // simulate blocks that do not trigger checkpoint update
				blocks <- types.Block{SlotNumber: 600} // simulate a block that triggers checkpoint update, but is still behind the DB head
				close(blocks)
				return blocks, func() {}, nil
			})
		// Only 600 block triggers checkpoint update
		lp.Filters.EXPECT().UpdateBackfillProgress(mock.Anything, types.Filter{ID: 1, StartingBlock: 100, Address: addr1}, int64(600), false).Return(nil).Once()
		// Once full range was processed filter must be marked as fully backfilled, even if we have not received block equal to DB head (e.g. due to sparse events)
		lp.Filters.EXPECT().UpdateBackfillProgress(mock.Anything, types.Filter{ID: 1, StartingBlock: 100, Address: addr1}, int64(1000), true).Return(nil).Once()

		err := lp.LogPoller.run(ctx)
		require.NoError(t, err)

		// Run 2: new filter inserted at 900 — must backfill [900, 1000] using DB watermark, not stop at in-memory 600.
		lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
		lp.Filters.EXPECT().GetFiltersToBackfill(int64(1000)).Return([]types.Filter{
			{ID: 2, StartingBlock: 900, Address: addr2},
		}, 900).Once()
		// The new filter's backfill should use the DB latest block (1000) as the upper bound
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, []types.PublicKey{addr2}, uint64(900), uint64(1000)).
			RunAndReturn(func(ctx context.Context, _ []types.PublicKey, _ uint64, _ uint64) (<-chan types.Block, func(), error) {
				blocks := make(chan types.Block)
				close(blocks)
				return blocks, func() {}, nil
			}).Once()
		lp.Filters.EXPECT().UpdateBackfillProgress(mock.Anything, types.Filter{ID: 2, StartingBlock: 900, Address: addr2}, int64(1000), true).Return(nil).Once()

		err = lp.LogPoller.run(ctx)
		require.NoError(t, err)
	})
}

// TestLogPoller_run_EncodedLogCollector_uniqueGetSignaturesRequests asserts that when LogPollerSlotsBatchSize is
// smaller than the number of slots in the backfill range, block fetching runs in multiple batches but
// GetSignaturesForAddress (getSlotsForAddressJob) is not invoked twice for the same RPC cursor (MinContextSlot + Before).
func TestLogPoller_run_EncodedLogCollector_uniqueGetSignaturesRequests(t *testing.T) {
	t.Parallel()

	const slotsBatchSize = 2
	cfg := config.NewDefault()
	cfg.Chain.LogPollerSlotsBatchSize = ptr[int64](slotsBatchSize)

	lp := newMockedLPwithConfig(t, cfg, chainID)
	loader := NewEncodedLogCollector(lp.Client, logger.Test(t), t.Name(), nil, nil, slotsBatchSize)
	require.NoError(t, loader.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, loader.Close()) })
	lp.LogPoller.loader = loader
	lp.LogPoller.lastProcessedSlot = 100

	ctx := t.Context()

	address, err := solana.PublicKeyFromBase58("J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4")
	require.NoError(t, err)
	addr := types.PublicKey(address)

	// Five slots 101..105 — strictly larger than slotsBatchSize so scheduleBlocksFetching uses multiple batches.
	slots := []uint64{105, 104, 103, 102, 101}

	lp.Filters.EXPECT().LoadFilters(mock.Anything).Return(nil).Once()
	lp.Filters.EXPECT().GetFiltersToBackfill(mock.Anything).Return(nil, 0).Once()
	lp.Filters.EXPECT().GetDistinctAddresses(mock.Anything).Return([]types.PublicKey{addr}, nil).Once()

	lp.Client.EXPECT().SlotHeightWithCommitment(mock.Anything, rpc.CommitmentFinalized).Return(uint64(105), nil).Once()

	lp.Client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
			txSigsResponse := make([]*rpc.TransactionSignature, 0, len(slots))
			for _, slot := range slots {
				tx := &rpc.TransactionSignature{Slot: slot}
				txSigsResponse = append(txSigsResponse, tx)
			}
			// add an extra signature with lower slot to signal that there the block range is fully processed
			txSigsResponse = append(txSigsResponse, &rpc.TransactionSignature{Slot: slots[len(slots)-1] - 1})
			return txSigsResponse, nil
		}).Once() // GetSignaturesForAddress should be called only once for the whole range, not once per batch.

	blockTime := solana.UnixTimeSeconds(128)
	lp.Client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			println(slot)
			require.Contains(t, slots, slot)
			return &rpc.GetBlockResult{
				Blockhash:   solana.Hash{1, 2, 3},
				BlockHeight: &slot,
				BlockTime:   &blockTime,
			}, nil
		}).Times(len(slots))

	err = lp.LogPoller.run(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(105), lp.LogPoller.lastProcessedSlot)
}

func Test_GetLastProcessedSlot(t *testing.T) {
	ctx := t.Context()

	type testCase struct {
		name              string
		lastProcessedSlot int64
		dbSlot            int64
		dbErr             error
		finalizedSlot     uint64
		firstAvailable    uint64
		lookbackErr       error
		expectedSlot      int64
		expectError       bool
	}

	testCases := []testCase{
		{
			name:              "uses lastProcessedSlot when greater than lookback",
			lastProcessedSlot: 12000,
			finalizedSlot:     11400, // so computed lookback = 11400 - 1000 = 10400 < 12000
			firstAvailable:    0,
			expectedSlot:      12000,
		},
		{
			name:           "uses dbSlot when greater than lookback",
			dbSlot:         11500,
			dbErr:          nil,
			finalizedSlot:  11100, // computed lookback = 10500
			firstAvailable: 0,
			expectedSlot:   11500,
		},
		{
			name:           "uses lookbackSlot when greater than dbSlot",
			dbSlot:         11000,
			dbErr:          nil,
			finalizedSlot:  13100, // computed lookback = 12500
			firstAvailable: 0,
			expectedSlot:   12100,
		},
		{
			name:           "uses lookbackSlot when db returns sql.ErrNoRows",
			dbErr:          sql.ErrNoRows,
			finalizedSlot:  10100, // lookback = 9100
			firstAvailable: 0,
			expectedSlot:   9100,
		},
		{
			name:        "returns error when DB returns unexpected error",
			dbErr:       errors.New("db failure"),
			expectError: true,
		},
		{
			name:        "returns error when computeLookbackWindow fails",
			dbSlot:      10000,
			dbErr:       nil,
			lookbackErr: errors.New("rpc error"),
			expectError: true,
		},
		{
			name:           "firstAvailableSlot overrides computed lookbackSlot",
			dbErr:          sql.ErrNoRows,
			finalizedSlot:  10600,
			firstAvailable: 10100, // should take precedence over computed lookback
			expectedSlot:   10100,
		},
	}

	cfg := config.NewDefault()
	cfg.Chain.BlockTime = commoncfg.MustNewDuration(600 * time.Millisecond)
	cfg.Chain.LogPollerStartingLookback = commoncfg.MustNewDuration(600 * time.Second)
	lp := newMockedLPwithConfig(t, cfg, chainID)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lp.LogPoller.lastProcessedSlot = tc.lastProcessedSlot
			if tc.lastProcessedSlot == 0 {
				lp.ORM.On("GetLatestBlock", mock.Anything).Return(tc.dbSlot, tc.dbErr).Once()

				// Set up lookback window mocks *only if GetLatestBlock is expected to succeed or be sql.ErrNoRows*
				shouldRunLookback := tc.dbErr == nil || errors.Is(tc.dbErr, sql.ErrNoRows)
				if shouldRunLookback {
					if tc.lookbackErr == nil {
						if tc.finalizedSlot != 0 {
							lp.Client.On("SlotHeightWithCommitment", mock.Anything, mock.Anything).
								Return(tc.finalizedSlot, nil).Once()
						}
						lp.Client.On("GetFirstAvailableBlock", mock.Anything).
							Return(tc.firstAvailable, nil).Once()
					} else {
						lp.Client.On("SlotHeightWithCommitment", mock.Anything, mock.Anything).
							Return(uint64(0), tc.lookbackErr).Once()
					}
				}
			}

			slot, err := lp.LogPoller.getLastProcessedSlot(ctx)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedSlot, slot)
				assert.NotZero(t, slot)
			}

			lp.ORM.AssertExpectations(t)
			lp.Client.AssertExpectations(t)
			lp.Filters.AssertExpectations(t)
		})
	}
}

func TestLogPoller_processBlocksRange(t *testing.T) {
	t.Parallel()

	t.Run("Returns error if failed to start backfill", func(t *testing.T) {
		lp := newMockedLP(t)
		expectedErr := errors.New("failed to start backfill")
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, expectedErr).Once()
		noopProgress := func(_ context.Context, _ int64) error { return nil }
		err := lp.LogPoller.processBlocksRange(t.Context(), nil, 10, 20, noopProgress)
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
		ctx, cancel := context.WithCancel(t.Context())
		lp := newMockedLP(t)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(context.Context, []types.PublicKey, uint64, uint64) (<-chan types.Block, func(), error) {
			cancel()
			return nil, funcWithCallExpectation(t), nil
		}).Once()
		noopProgress := func(_ context.Context, _ int64) error { return nil }
		err := lp.LogPoller.processBlocksRange(ctx, nil, 10, 20, noopProgress)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("Happy path", func(t *testing.T) {
		lp := newMockedLP(t)
		blocks := make(chan types.Block, 2)
		blocks <- types.Block{SlotNumber: 11}
		blocks <- types.Block{SlotNumber: 12}
		close(blocks)
		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(blocks, funcWithCallExpectation(t), nil).Once()
		noopProgress := func(_ context.Context, _ int64) error { return nil }
		err := lp.LogPoller.processBlocksRange(t.Context(), nil, 10, 20, noopProgress)
		require.NoError(t, err)
	})
	t.Run("Updates lastProcessedSlot incrementally on partial failure", func(t *testing.T) {
		lp := newMockedLP(t)
		blocks := make(chan types.Block, 3)
		blocks <- types.Block{SlotNumber: 11}
		blocks <- types.Block{SlotNumber: 12}

		expectedErr := errors.New("simulated processing error")
		callCount := 0
		lp.LogPoller.processBlocks = func(_ context.Context, _ []types.Block) error {
			callCount++
			if callCount == 1 {
				blocks <- types.Block{SlotNumber: 13}
				close(blocks)
				return nil
			}
			return expectedErr
		}

		lp.Loader.EXPECT().BackfillForAddresses(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(blocks, funcWithCallExpectation(t), nil).Once()
		noopProgress := func(_ context.Context, _ int64) error { return nil }
		err := lp.LogPoller.processBlocksRange(t.Context(), nil, 10, 20, noopProgress)
		require.ErrorIs(t, err, expectedErr)
	})
}

func TestProcess(t *testing.T) {
	ctx := t.Context()

	addr := newRandomPublicKey(t)
	eventName := "myEvent"
	eventSig := types.NewEventSignatureFromName(eventName)
	event := struct {
		A int64
		B string
	}{55, "hello"}
	subKeyValA, err := types.NewIndexedValue(event.A)
	require.NoError(t, err)
	subKeyValB, err := types.NewIndexedValue(event.B)
	require.NoError(t, err)

	filterID := rand.Int63()
	chainID := uuid.NewString()

	txIndex := int(rand.Int31())
	txLogIndex := uint(rand.Uint32())

	expectedLog := newRandomLog(t, filterID, chainID, eventName)
	expectedLog.Address = addr
	expectedLog.LogIndex, err = makeLogIndex(txIndex, txLogIndex)
	require.NoError(t, err)
	expectedLog.SubkeyValues = []types.IndexedValue{subKeyValA, subKeyValB}

	expectedLog.Data, err = bin.MarshalBorsh(&event)
	require.NoError(t, err)

	expectedLog.Data = append(eventSig[:], expectedLog.Data...)
	ev := types.ProgramEvent{
		Program: addr.ToSolana().String(),
		BlockData: types.BlockData{
			SlotNumber:          uint64(expectedLog.BlockNumber),
			BlockHeight:         3,
			BlockHash:           expectedLog.BlockHash.ToSolana(),
			BlockTime:           solana.UnixTimeSeconds(expectedLog.BlockTimestamp.Unix()),
			TransactionHash:     expectedLog.TxHash.ToSolana(),
			TransactionIndex:    txIndex,
			TransactionLogIndex: txLogIndex,
			Error:               nil,
		},
		Data: base64.StdEncoding.EncodeToString(expectedLog.Data),
	}

	orm := mocks.NewMockORM(t)
	cl := mocks.NewRPCClient(t)
	lggr := logger.Sugared(logger.Test(t))
	lp, err := New(lggr, orm, cl, config.NewDefault(), chainID)
	require.NoError(t, err)

	var idlTypeInt64 codecv1.IdlType
	var idlTypeString codecv1.IdlType

	err = json.Unmarshal([]byte("\"i64\""), &idlTypeInt64)
	require.NoError(t, err)
	err = json.Unmarshal([]byte("\"string\""), &idlTypeString)
	require.NoError(t, err)

	idl := types.EventIdl{
		Event: codecv1.IdlEvent{
			Name: "myEvent",
			Fields: []codecv1.IdlEventField{{
				Name: "A",
				Type: idlTypeInt64,
			}, {
				Name: "B",
				Type: idlTypeString,
			}},
		},
		Types: []codecv1.IdlTypeDef{},
	}

	filter := types.Filter{
		Name:        "test filter",
		EventName:   eventName,
		Address:     addr,
		EventSig:    eventSig,
		EventIdl:    idl,
		SubkeyPaths: [][]string{{"A"}, {"B"}},
	}
	orm.EXPECT().ChainID().Return(chainID).Maybe()
	orm.EXPECT().SelectFilters(mock.Anything).Return([]types.Filter{filter}, nil).Once()
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{}, nil).Once()
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, f types.Filter) (int64, error) {
		require.Equal(t, f, filter)
		return filterID, nil
	}).Once()

	err = lp.RegisterFilter(ctx, filter)
	require.NoError(t, err)

	var nextSeqNum int64

	t.Run("accepts matching log", func(t *testing.T) {
		nextSeqNum++
		want := expectedLog
		want.SequenceNum = nextSeqNum

		orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, logs []types.Log) error {
			require.Len(t, logs, 1)
			assert.Equal(t, want, logs[0])
			return nil
		}).Once()
		err = lp.Process(ctx, ev)
		assert.NoError(t, err)
	})

	t.Run("does not advance sequence number when insert fails", func(t *testing.T) {
		fl := lp.filters.(*filters)

		insertErr := errors.New("insert failed")
		orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, logs []types.Log) error {
			require.Len(t, logs, 1)
			assert.Equal(t, nextSeqNum+1, logs[0].SequenceNum, "insert should still receive the staged sequence number")
			return insertErr
		}).Once()
		err = lp.Process(ctx, ev)
		require.ErrorIs(t, err, insertErr)
		assert.Equal(t, nextSeqNum, fl.seqNums[filterID], "in-memory counter must not advance after failed insert")

		nextSeqNum++
		want := expectedLog
		want.SequenceNum = nextSeqNum

		orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, logs []types.Log) error {
			require.Len(t, logs, 1)
			assert.Equal(t, want, logs[0])
			return nil
		}).Once()
		err = lp.Process(ctx, ev)
		assert.NoError(t, err)
	})

	t.Run("populates expiresAt field when retention is set", func(t *testing.T) {
		filter.Retention = 30 * time.Minute
		orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, f types.Filter) (int64, error) {
			require.Equal(t, f, filter)
			return filterID, nil
		}).Once()
		err = lp.RegisterFilter(ctx, filter)
		require.NoError(t, err)

		orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, logs []types.Log) error {
			require.Len(t, logs, 1)
			log := logs[0]
			assert.Less(t, time.Until(*log.ExpiresAt), 30*time.Minute) // should be slightly less than 30 minutes from now
			assert.Greater(t, time.Until(*log.ExpiresAt), 29*time.Minute)
			nextSeqNum++
			assert.Equal(t, nextSeqNum, log.SequenceNum)
			return nil
		}).Once()
		err = lp.Process(ctx, ev)
		assert.NoError(t, err)
		filter.Retention = 0
	})

	jsonErr := []byte("{\"InstructionError\":[2,{\"Custom\":6001}]}")
	err = json.Unmarshal(jsonErr, &ev.Error)
	require.NoError(t, err)

	t.Run("ignores reverted log when IncludeReverted = false", func(t *testing.T) {
		// Should ignore this log, since reverted logs are not included. Should not call InsertLogs
		err = lp.Process(ctx, ev)
		assert.NoError(t, err)
	})

	filter.IncludeReverted = true
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, f types.Filter) (int64, error) {
		require.Equal(t, f, filter)
		return filterID, nil
	}).Once()
	err = lp.RegisterFilter(ctx, filter)
	require.NoError(t, err)

	t.Run("accepts reverted log when IncludeReverted = true", func(t *testing.T) {
		nextSeqNum++
		want := expectedLog
		want.SequenceNum = nextSeqNum
		want.Error = new(string)
		*want.Error = string(jsonErr)

		orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, logs []types.Log) error {
			require.Len(t, logs, 1)
			assert.Equal(t, want, logs[0])
			return nil
		}).Once()

		err = lp.Process(ctx, ev)
		assert.NoError(t, err)
	})

	orm.EXPECT().MarkFilterDeleted(mock.Anything, mock.Anything).Return(nil).Once()
	err = lp.UnregisterFilter(ctx, filter.Name)
	require.NoError(t, err)

	t.Run("ignores non-matching logs", func(t *testing.T) {
		err = lp.Process(ctx, ev)
		assert.NoError(t, err)

		ev.Error = nil
		err = lp.Process(ctx, ev)
		assert.NoError(t, err)
	})
}

func Test_LogPoller_Replay(t *testing.T) {
	t.Parallel()
	fromBlock := int64(5)

	lp := newMockedLP(t)
	assertReplayInfo := func(requestBlock int64, status types.ReplayStatus) {
		assert.Equal(t, requestBlock, lp.LogPoller.replay.requestBlock)
		assert.Equal(t, status, lp.LogPoller.replay.status)
	}

	t.Run("ReplayInfo state initialized properly", func(t *testing.T) {
		assertReplayInfo(0, types.ReplayStatusNoRequest)
	})

	t.Run("ordinary replay request", func(t *testing.T) {
		lp.Filters.EXPECT().UpdateStartingBlocks(fromBlock).Once()
		lp.LogPoller.Replay(fromBlock)
		assertReplayInfo(fromBlock, types.ReplayStatusRequested)
	})

	t.Run("redundant replay request", func(t *testing.T) {
		lp.LogPoller.replay.requestBlock = fromBlock
		lp.LogPoller.replay.status = types.ReplayStatusRequested
		lp.LogPoller.Replay(fromBlock + 10)
		assertReplayInfo(fromBlock, types.ReplayStatusRequested)
	})

	t.Run("replay request updated", func(t *testing.T) {
		lp.LogPoller.replay.status = types.ReplayStatusNoRequest
		lp.Filters.EXPECT().UpdateStartingBlocks(fromBlock - 1).Once()
		lp.LogPoller.Replay(fromBlock - 1)
		assertReplayInfo(fromBlock-1, types.ReplayStatusRequested)
	})

	t.Run("replay request updated while pending", func(t *testing.T) {
		lp.LogPoller.replay.requestBlock = fromBlock
		lp.LogPoller.replay.status = types.ReplayStatusPending
		lp.Filters.EXPECT().UpdateStartingBlocks(fromBlock - 1).Once()
		lp.LogPoller.Replay(fromBlock - 1)
		assertReplayInfo(fromBlock-1, types.ReplayStatusPending)
	})

	t.Run("checkForReplayRequest should not enter pending state if there are no requests", func(t *testing.T) {
		lp.LogPoller.replay.requestBlock = 400
		lp.LogPoller.replay.status = types.ReplayStatusComplete
		assert.False(t, lp.LogPoller.checkForReplayRequest())
		assertReplayInfo(400, types.ReplayStatusComplete)
		assert.Equal(t, types.ReplayStatusComplete, lp.LogPoller.ReplayStatus())
	})

	t.Run("checkForReplayRequest should enter pending state if there is a new request", func(t *testing.T) {
		lp.LogPoller.replay.status = types.ReplayStatusRequested
		lp.LogPoller.replay.requestBlock = 18
		assert.True(t, lp.LogPoller.checkForReplayRequest())
		assertReplayInfo(18, types.ReplayStatusPending)
		assert.Equal(t, types.ReplayStatusPending, lp.LogPoller.ReplayStatus())
	})

	t.Run("replayComplete enters ReplayComplete state", func(t *testing.T) {
		lp.LogPoller.replay.requestBlock = 10
		lp.LogPoller.replay.status = types.ReplayStatusPending
		lp.LogPoller.replayComplete(8, 20)
		assertReplayInfo(10, types.ReplayStatusComplete)
	})

	t.Run("replayComplete stays in pending state if lower block request received", func(t *testing.T) {
		lp.LogPoller.replay.requestBlock = 3
		lp.LogPoller.replay.status = types.ReplayStatusPending
		lp.LogPoller.replayComplete(8, 20)
		assertReplayInfo(3, types.ReplayStatusRequested)
	})
}

func TestShuffledFilters(t *testing.T) {
	fl := &filters{
		filtersByID: map[int64]*types.Filter{
			0: {Name: "Filter A"},
			1: {Name: "Filter B"},
			2: {Name: "Filter C"},
		},
	}

	seen := map[string]bool{}
	for filter := range fl.shuffledFilters() {
		seen[filter.Name] = true
	}

	require.Len(t, seen, 3)

	for _, filter := range fl.filtersByID {
		assert.Contains(t, seen, filter.Name)
	}
}

func TestBackgroundWorkerRun(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	lggr := logger.TestSugared(t)
	orm := mocks.NewMockORM(t)
	cl := mocks.NewRPCClient(t)
	lp, err := New(lggr, orm, cl, config.NewDefault(), chainID)
	require.NoError(t, err)

	addr := newRandomPublicKey(t)
	filter1 := decoderReadyTestFilter(t, 1, "Filter A", "TestEvent", addr)
	filter2 := decoderReadyTestFilter(t, 2, "Filter B", "TestItem", addr)
	filter3 := decoderReadyTestFilter(t, 3, "Filter C", "TestEvent", addr)

	filters := []types.Filter{
		filter1, filter2, filter3,
	}

	orm.EXPECT().SelectFilters(mock.Anything).Return(filters, nil).Once()
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{}, nil).Once()
	orm.EXPECT().PruneLogsForFilter(mock.Anything, mock.Anything).Return(int64(1), nil).Times(3)

	lp.backgroundWorkerRun(ctx)
	orm.AssertNumberOfCalls(t, "PruneLogsForFilter", 3)
}

func newRandomPublicKey(t *testing.T) types.PublicKey {
	t.Helper()
	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	return types.PublicKey(pubKey)
}

func newRandomEventSignature(t *testing.T) types.EventSignature {
	t.Helper()
	pubKey := newRandomPublicKey(t)
	return types.EventSignature(pubKey[:8])
}

func newRandomLog(t *testing.T, filterID int64, chainID string, eventName string) types.Log {
	t.Helper()
	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	data := []byte("solana is fun")
	signature, err := privateKey.Sign(data)
	require.NoError(t, err)
	return types.Log{
		FilterID:       filterID,
		ChainID:        chainID,
		LogIndex:       rand.Int63n(1000),
		BlockHash:      types.Hash(pubKey),
		BlockNumber:    rand.Int63n(1000000),
		BlockTimestamp: time.Unix(1731590113, 0).UTC(),
		Address:        types.PublicKey(pubKey),
		EventSig:       types.NewEventSignatureFromName(eventName),
		SubkeyValues:   []types.IndexedValue{{3, 2, 1}, {1}, {1, 2}, pubKey.Bytes()},
		TxHash:         types.Signature(signature),
		Data:           data,
		SequenceNum:    rand.Int63n(500),
	}
}
