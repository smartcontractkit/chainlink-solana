package logpoller

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func TestBlocksSorter(t *testing.T) {
	t.Parallel()
	t.Run("Properly closes even if there is still work to do", func(t *testing.T) {
		ctx := t.Context()
		sorter, ch := newBlocksSorter(make(chan types.Block), logger.Test(t), []uint64{1, 2})
		require.NoError(t, sorter.Start(ctx))
		require.NoError(t, sorter.Close())
		select {
		case <-ch:
			require.Fail(t, "expected channel to remain open as not all work was done")
		default:
		}
	})
	t.Run("Writes blocks in specified order defined by expectedBlocks", func(t *testing.T) {
		ctx := t.Context()
		inCh := make(chan types.Block)
		expectedBlocks := []uint64{1, 2, 10, 3}
		sorter, ch := newBlocksSorter(inCh, logger.Test(t), expectedBlocks)
		require.NoError(t, sorter.Start(ctx))
		t.Cleanup(func() {
			require.NoError(t, sorter.Close())
		})
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, b := range []uint64{2, 10, 1, 3} {
				inCh <- types.Block{SlotNumber: b}
			}
			close(inCh)
		}()
		for _, b := range expectedBlocks {
			select {
			case block, ok := <-ch:
				require.True(t, ok)
				require.Equal(t, b, block.SlotNumber)
			case <-ctx.Done():
				require.Fail(t, "expected to receive all blocks, before timeout")
			}
		}

		select {
		case _, ok := <-ch:
			require.False(t, ok)
		case <-ctx.Done():
			require.Fail(t, "expected channel to be closed")
		}
	})
	t.Run("Terminal flush with aborted head does not block rest of the queue", func(t *testing.T) {
		ctx := t.Context()
		inCh := make(chan types.Block)
		expectedBlocks := []uint64{1, 2, 3}
		sorter, ch := newBlocksSorter(inCh, logger.Test(t), expectedBlocks)

		// Pre-seed: all three blocks already ready before the sorter starts.
		// This simulates arrivals whose signals coalesced into one wakeup.
		sorter.readyBlocks[1] = types.Block{SlotNumber: 1, Aborted: true}
		sorter.readyBlocks[2] = types.Block{SlotNumber: 2}
		sorter.readyBlocks[3] = types.Block{SlotNumber: 3}

		require.NoError(t, sorter.Start(ctx))
		t.Cleanup(func() { require.NoError(t, sorter.Close()) })

		// Close input: readBlocks closes receivedNewBlock → exactly one terminal
		// flush. It pops aborted head 1 and returns early; 2 and 3 stay queued.
		close(inCh)

		for _, b := range []uint64{2, 3} {
			select {
			case block := <-ch:
				require.Equal(t, b, block.SlotNumber)
			case <-time.After(5 * time.Second):
				require.Fail(t, "blocks stuck behind aborted head on terminal flush")
			}
		}
	})
}
