package logpoller

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestBlocksSorter(t *testing.T) {
	t.Parallel()
	t.Run("Properly closes even if there is still work to do", func(t *testing.T) {
		ctx := tests.Context(t)
		sorter, ch := newBlocksSorter(make(chan Block), logger.Test(t), []uint64{1, 2})
		require.NoError(t, sorter.Start(ctx))
		require.NoError(t, sorter.Close())
		select {
		case <-ch:
			require.Fail(t, "expected channel to remain open as not all work was done")
		default:
		}
	})
	t.Run("Writes blocks in specified order defined by expectedBlocks", func(t *testing.T) {
		ctx := tests.Context(t)
		inCh := make(chan Block)
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
				inCh <- Block{SlotNumber: b}
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
}
