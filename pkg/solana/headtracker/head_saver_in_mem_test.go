package headtracker_test

import (
	"sync"
	"testing"

	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/cltest"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker"
	"github.com/stretchr/testify/require"
)

func configureInMemorySaver(t *testing.T) *headtracker.HeadSaver {
	htCfg := headtracker.NewConfig()
	lggr, _ := logger.New()
	return headtracker.NewSaver(htCfg, lggr)
}

func TestInMemoryHeadSaver_Save(t *testing.T) {
	t.Parallel()
	saver := configureInMemorySaver(t)

	t.Run("happy path, saving heads", func(t *testing.T) {
		head := cltest.Head(1)
		err := saver.Save(testutils.Context(t), head)
		require.NoError(t, err)

		latest := saver.LatestChain()
		require.NoError(t, err)
		require.Equal(t, int64(1), latest.BlockNumber())

		latest = saver.LatestChain()
		require.NotNil(t, latest)
		require.Equal(t, int64(1), latest.BlockNumber())

		latest = saver.Chain(head.BlockHash())
		require.NotNil(t, latest)
		require.Equal(t, int64(1), latest.BlockNumber())

		// Add more heads
		head = cltest.Head(2)
		err = saver.Save(testutils.Context(t), head)
		require.NoError(t, err)
		head = cltest.Head(3)
		err = saver.Save(testutils.Context(t), head)
		require.NoError(t, err)

		latest = saver.LatestChain()
		require.Equal(t, int64(3), latest.BlockNumber())

		// Check total number of heads
		require.Equal(t, 3, len(saver.Heads))
	})

	t.Run("save invalid head", func(t *testing.T) {
		err := saver.Save(testutils.Context(t), nil)
		require.Error(t, err)
	})

	t.Run("saving heads with same block number", func(t *testing.T) {
		head := cltest.Head(4)
		err := saver.Save(testutils.Context(t), head)
		require.NoError(t, err)

		head = cltest.Head(4)
		err = saver.Save(testutils.Context(t), head)
		require.NoError(t, err)

		head = cltest.Head(4)
		err = saver.Save(testutils.Context(t), head)
		require.NoError(t, err)

		latest := saver.LatestChain()
		require.NoError(t, err)
		require.Equal(t, int64(4), latest.BlockNumber())

		headsWithSameNumber := len(saver.HeadByNumber(4))
		require.Equal(t, 3, headsWithSameNumber)
	})
	t.Run("concurrent calls to Save", func(t *testing.T) {
		var wg sync.WaitGroup
		numRoutines := 10
		wg.Add(numRoutines)

		for i := 1; i <= numRoutines; i++ {
			go func(num int) {
				defer wg.Done()
				head := cltest.Head(num)
				err := saver.Save(testutils.Context(t), head)
				require.NoError(t, err)
			}(i)
		}

		wg.Wait()

		latest := saver.LatestChain()
		require.Equal(t, int64(numRoutines), latest.BlockNumber())
	})
}

func TestInMemoryHeadSaver_TrimOldHeads(t *testing.T) {
	t.Parallel()
	saver := configureInMemorySaver(t)

	t.Run("happy path, trimming old heads", func(t *testing.T) {
		// Save heads with block numbers 1, 2, 3, and 4
		for i := 1; i <= 4; i++ {
			head := cltest.Head(i)
			err := saver.Save(testutils.Context(t), head)
			require.NoError(t, err)
		}

		require.Equal(t, 4, len(saver.Heads))

		// Trim old heads, keeping only the last 3 blocks
		saver.TrimOldHeads(3)

		// Check that the correct heads remain
		require.Equal(t, 3, len(saver.Heads))
		require.Equal(t, 1, len(saver.HeadByNumber(3)))
		require.Equal(t, 1, len(saver.HeadByNumber(4)))
		require.Equal(t, 0, len(saver.HeadByNumber(1)))

		// Check that the latest head is correct
		latest := saver.LatestChain()
		require.Equal(t, int64(4), latest.BlockNumber())

		// Clear All Heads
		saver.TrimOldHeads(0)
		require.Equal(t, 0, len(saver.Heads))
		require.Equal(t, 0, len(saver.HeadsNumber))
	})

	t.Run("error path, block number lower than highest chain", func(t *testing.T) {
		for i := 1; i <= 4; i++ {
			head := cltest.Head(i)
			err := saver.Save(testutils.Context(t), head)
			require.NoError(t, err)
		}

		saver.TrimOldHeads(4)

		// Check that no heads are removed
		require.Equal(t, 4, len(saver.Heads))
		require.Equal(t, 4, len(saver.HeadsNumber))

		latest := saver.LatestChain()
		require.Equal(t, int64(4), latest.BlockNumber())
	})

	t.Run("concurrent calls to TrimOldHeads", func(t *testing.T) {
		// Save heads with block numbers 1, 2, 3, and 4
		for i := 1; i <= 4; i++ {
			head := cltest.Head(i)
			err := saver.Save(testutils.Context(t), head)
			require.NoError(t, err)
		}

		// Concurrently add multiple heads with different block numbers
		var wg sync.WaitGroup
		wg.Add(4)
		for i := 5; i <= 8; i++ {
			go func(num int) {
				defer wg.Done()
				head := cltest.Head(num)
				err := saver.Save(testutils.Context(t), head)
				require.NoError(t, err)
			}(i)
		}
		wg.Wait()

		// Concurrently trim old heads of depth 3
		wg.Add(1)
		go func() {
			defer wg.Done()
			saver.TrimOldHeads(3)
		}()
		wg.Wait()

		// Check that the correct heads remain after concurrent calls to TrimOldHeads
		require.Equal(t, 3, len(saver.Heads))
		require.Equal(t, 1, len(saver.HeadByNumber(7)))
		require.Equal(t, 1, len(saver.HeadByNumber(8)))
		require.Equal(t, 0, len(saver.HeadByNumber(1)))

		latest := saver.LatestChain()
		require.Equal(t, int64(8), latest.BlockNumber())
	})
}

func TestInMemoryHeadSaver_Chain(t *testing.T) {
	t.Parallel()
	saver := configureInMemorySaver(t)

	t.Run("happy path, valid block hash", func(t *testing.T) {
		head1 := cltest.Head(1)
		head2 := cltest.Head(2)
		err := saver.Save(testutils.Context(t), head1)
		require.NoError(t, err)
		err = saver.Save(testutils.Context(t), head2)
		require.NoError(t, err)

		retrievedHead1 := saver.Chain(head1.BlockHash())
		retrievedHead2 := saver.Chain(head2.BlockHash())

		require.Equal(t, head1, retrievedHead1)
		require.Equal(t, head2, retrievedHead2)

	})

	t.Run("invalid block hash", func(t *testing.T) {
		head1 := cltest.Head(1)
		err := saver.Save(testutils.Context(t), head1)
		require.NoError(t, err)
		head2 := cltest.Head(2)
		err = saver.Save(testutils.Context(t), head2)
		require.NoError(t, err)

		saver.TrimOldHeads(1)

		invalidBlockHash := head1.BlockHash()
		retrievedHead := saver.Chain(invalidBlockHash)

		require.Nil(t, retrievedHead)
	})
}

func TestInMemoryHeadSaver_LatestChain(t *testing.T) {
	t.Parallel()
	saver := configureInMemorySaver(t)

	t.Run("happy path", func(t *testing.T) {
		// Save a valid head
		head := cltest.Head(1)
		err := saver.Save(testutils.Context(t), head)
		require.NoError(t, err)

		latest := saver.LatestChain()
		require.Equal(t, int64(1), latest.BlockNumber())
	})
}
