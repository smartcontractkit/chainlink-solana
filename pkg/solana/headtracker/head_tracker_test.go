package headtracker_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"testing"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/onsi/gomega"

	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	commonmocks "github.com/smartcontractkit/chainlink-relay/pkg/types/mocks"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/cltest"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"

	"github.com/stretchr/testify/mock"
	"github.com/test-go/testify/assert"
	"github.com/test-go/testify/require"
)

// Why do we need this?
// Allow us to retreive the earliest head in our HeadSaver
func firstHead(
	t *testing.T,
	hs *headtracker.InMemoryHeadSaver[
		*types.Head,
		types.Hash,
		types.ChainID,
	]) (h *types.Head) {

	// Get all the Heads in the HeadSaver and find the one with lowest block number
	// Iterate over HeadsNumber
	// HeadsNumber is a map[int64][]H
	lowestBlockNumber := int64(math.MaxInt64)
	for blockNumber := range hs.HeadsNumber {
		if blockNumber < lowestBlockNumber {
			lowestBlockNumber = blockNumber
		}
	}

	return hs.HeadsNumber[lowestBlockNumber][0]
}

func TestHeadTracker_New(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	client := cltest.NewClientMockWithDefaultChain(t)
	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(cltest.Head(0), nil)
	headSaver := headtracker.NewSaver(cfg, lggr)
	assert.Nil(t, headSaver.Save(testutils.Context(t), cltest.Head(1)))
	last := cltest.Head(16)
	assert.Nil(t, headSaver.Save(testutils.Context(t), last))
	assert.Nil(t, headSaver.Save(testutils.Context(t), cltest.Head(10)))

	ht := createHeadTracker(t, cfg, client, headSaver)
	ht.Start(t)

	latest := ht.headSaver.LatestChain()
	require.NotNil(t, latest)
	assert.Equal(t, last.BlockNumber(), latest.BlockNumber())
}

// // The function `TestHeadTracker_Save_InsertsAndTrimsTable` tests the functionality of inserting and
// // trimming a table in the head tracker.
func TestHeadTracker_Save_InsertsAndTrimsTable(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)

	client := cltest.NewClientMockWithDefaultChain(t)

	// Generate 200 consecutive heads
	for idx := 0; idx < 200; idx++ {
		idxHead := cltest.Head(idx)
		parentHead := headSaver.LatestChain()

		if parentHead != nil {
			idxHead.Parent = parentHead
		}
		parentHead = idxHead
		assert.Nil(t, headSaver.Save(testutils.Context(t), idxHead))
	}

	ht := createHeadTracker(t, headtracker.NewConfig(), client, headSaver)

	h := cltest.Head(200)
	h.Parent = headSaver.LatestChain()
	require.NoError(t, ht.headSaver.Save(testutils.Context(t), h))
	assert.Equal(t, int64(200), ht.headSaver.LatestChain().BlockNumber())

	firstHead := firstHead(t, headSaver)

	assert.Equal(t, int64(100), firstHead.BlockNumber())

	lastHead := headSaver.LatestChain()
	assert.Equal(t, int64(200), lastHead.BlockNumber())
}

func TestHeadTracker_Get(t *testing.T) {
	t.Parallel()

	start := cltest.Head(5)

	tests := []struct {
		name    string
		initial *types.Head
		toSave  *types.Head
		want    int64
	}{
		{"greater", start, cltest.Head(6), int64(6)},
		{"less than", start, cltest.Head(1), int64(5)},
		{"zero", start, cltest.Head(0), int64(5)},
		{"nil", start, nil, int64(5)},
		{"nil no initial", nil, nil, int64(0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lggr, _ := logger.New()
			cfg := headtracker.NewConfig()
			headSaver := headtracker.NewSaver(cfg, lggr)
			client := cltest.NewClientMockWithDefaultChain(t)
			mockChain := &testutils.MockChain{Client: client}
			chStarted := make(chan struct{})

			client.On("SubscribeNewHead", mock.Anything, mock.Anything).
				Maybe().
				Return(
					func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription {
						defer close(chStarted)
						return mockChain.NewSub(t)
					},
					func(ctx context.Context, ch chan<- *types.Head) error { return nil },
				)
			client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(cltest.Head(0), nil)

			fnCall := client.On("HeadByNumber", mock.Anything, mock.Anything)
			fnCall.RunFn = func(args mock.Arguments) {
				num := args.Get(1).(*big.Int)
				fnCall.ReturnArguments = mock.Arguments{cltest.Head(num), nil}
			}

			if test.initial != nil {
				assert.Nil(t, headSaver.Save(testutils.Context(t), test.initial))
			}

			ht := createHeadTracker(t, cfg, client, headSaver)
			ht.Start(t)

			if test.toSave != nil {
				err := ht.headSaver.Save(testutils.Context(t), test.toSave)
				assert.NoError(t, err)
			}

			// Check if that is the correct head that we want
			assert.Equal(t, test.want, ht.headSaver.LatestChain().BlockNumber())
		})
	}
}

func TestHeadTracker_Start_NewHeads(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	chStarted := make(chan struct{})
	mockChain := &testutils.MockChain{Client: client}

	sub := mockChain.NewSub(t)
	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(cltest.Head(0), nil)
	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) {
			close(chStarted)
		}).
		Return(sub, nil)

	ht := createHeadTracker(t, cfg, client, headSaver)
	ht.Start(t)

	<-chStarted
}

func TestHeadTracker_Start_CancelContext(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	chStarted := make(chan struct{})
	mockChain := &testutils.MockChain{Client: client}

	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			assert.FailNow(t, "context was not cancelled within 10s")
		}
	}).Return(cltest.Head(0), nil)
	sub := mockChain.NewSub(t)
	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) {
			close(chStarted)
		}).
		Return(sub, nil).
		Maybe()

	ht := createHeadTracker(t, cfg, client, headSaver)
	ctx, cancel := context.WithCancel(testutils.Context(t))
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()
	err := ht.headTracker.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, ht.headTracker.Close())
}

func TestHeadTracker_CallsHeadTrackableCallbacks(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	mockChain := &testutils.MockChain{Client: client}
	chchHeaders := make(chan testutils.RawSub[*types.Head], 1)

	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription {
				sub := mockChain.NewSub(t)
				chchHeaders <- testutils.NewRawSub(ch, sub.Err())
				return sub
			},
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)
	client.On("HeadByNumber", mock.Anything, mock.Anything).Return(cltest.Head(0), nil)

	checker := &cltest.MockHeadTrackable{}
	ht := createHeadTrackerWithChecker(t, cfg, client, headSaver, checker)
	ht.Start(t)
	assert.Equal(t, int32(0), checker.OnNewLongestChainCount())

	headers := <-chchHeaders
	headers.TrySend(cltest.Head(1))

	g.Eventually(checker.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))

	ht.Stop(t)
	assert.Equal(t, int32(1), checker.OnNewLongestChainCount())
}

func TestHeadTracker_ReconnectOnError(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	mockChain := &testutils.MockChain{Client: client}

	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription { return mockChain.NewSub(t) },
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)
	client.On("SubscribeNewHead", mock.Anything, mock.Anything).Return(nil, errors.New("cannot reconnect"))
	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription { return mockChain.NewSub(t) },
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)
	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(cltest.Head(0), nil)

	checker := &cltest.MockHeadTrackable{}
	ht := createHeadTrackerWithChecker(t, cfg, client, headSaver, checker)

	// connect
	ht.Start(t)
	assert.Equal(t, int32(0), checker.OnNewLongestChainCount())

	// trigger reconnect loop
	mockChain.SubsErr(errors.New("test error to force reconnect"))
	g.SetDefaultEventuallyTimeout(2 * time.Second)
	g.Eventually(checker.OnNewLongestChainCount).Should(gomega.Equal(int32(1)))
}

func TestHeadTracker_ResubscribeOnSubscriptionError(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	mockChain := &testutils.MockChain{Client: client}
	chchHeaders := make(chan testutils.RawSub[*types.Head], 1)

	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription {
				sub := mockChain.NewSub(t)
				chchHeaders <- testutils.NewRawSub(ch, sub.Err())
				return sub
			},
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)
	client.On("HeadByNumber", mock.Anything, mock.Anything).Return(cltest.Head(0), nil)

	checker := &cltest.MockHeadTrackable{}
	ht := createHeadTrackerWithChecker(t, cfg, client, headSaver, checker)

	ht.Start(t)
	assert.Equal(t, int32(0), checker.OnNewLongestChainCount())

	headers := <-chchHeaders
	go func() {
		headers.TrySend(cltest.Head(1))
	}()

	g.Eventually(func() bool {
		report := ht.headTracker.HealthReport()
		return !slices.ContainsFunc(maps.Values(report), func(e error) bool { return e != nil })
	}, 5*time.Second, testutils.TestInterval).Should(gomega.Equal(true))

	// trigger reconnect loop
	headers.CloseCh()

	// wait for full disconnect and a new subscription
	g.Eventually(checker.OnNewLongestChainCount, 5*time.Second, testutils.TestInterval).Should(gomega.Equal(int32(1)))
}

func TestHeadTracker_Start_LoadsLatestChain(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	mockChain := &testutils.MockChain{Client: client}
	chchHeaders := make(chan testutils.RawSub[*types.Head], 1)

	heads := []*types.Head{
		cltest.Head(0),
		cltest.Head(1),
		cltest.Head(2),
		cltest.Head(3),
	}

	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(heads[3], nil).Maybe()
	client.On("HeadByNumber", mock.Anything, big.NewInt(2)).Return(heads[2], nil).Maybe()
	client.On("HeadByNumber", mock.Anything, big.NewInt(1)).Return(heads[1], nil).Maybe()
	client.On("HeadByNumber", mock.Anything, big.NewInt(0)).Return(heads[0], nil).Maybe()
	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription {
				sub := mockChain.NewSub(t)
				chchHeaders <- testutils.NewRawSub(ch, sub.Err())
				return sub
			},
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)

	trackable := &cltest.MockHeadTrackable{}
	ht := createHeadTrackerWithChecker(t, cfg, client, headSaver, trackable)

	require.NoError(t, headSaver.Save(testutils.Context(t), heads[2]))

	ht.Start(t)

	assert.Equal(t, int32(0), trackable.OnNewLongestChainCount())

	headers := <-chchHeaders
	go func() {
		headers.TrySend(cltest.Head(1))
	}()

	gomega.NewWithT(t).Eventually(func() bool {
		report := ht.headTracker.HealthReport()
		maps.Copy(report, ht.headBroadcaster.HealthReport())
		return !slices.ContainsFunc(maps.Values(report), func(e error) bool { return e != nil })
	}, 5*time.Second, testutils.TestInterval).Should(gomega.Equal(true))

	h := headSaver.LatestChain()
	require.NotNil(t, h)
	assert.Equal(t, h.BlockNumber(), int64(3))
}

func TestHeadTracker_SwitchesToLongestChainWithHeadSamplingEnabled(t *testing.T) {
	t.Parallel()

	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	mockChain := &testutils.MockChain{Client: client}
	chchHeaders := make(chan testutils.RawSub[*types.Head], 1)
	checker := commonmocks.NewHeadTrackable[*types.Head, types.Hash](t)
	ht := createHeadTrackerWithChecker(t, cfg, client, headSaver, checker)
	cfg.SetHeadTrackerSamplingInterval(2500 * time.Millisecond)

	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription {
				sub := mockChain.NewSub(t)
				chchHeaders <- testutils.NewRawSub(ch, sub.Err())
				return sub
			},
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)

	// ---------------------
	blocks := cltest.NewBlocks(t, 10)

	head0 := blocks.Head(0)
	// Initial query
	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(head0, nil)
	ht.Start(t)

	headSeq := cltest.NewHeadBuffer(t)
	headSeq.Append(blocks.Head(0))
	headSeq.Append(blocks.Head(1))

	// Blocks 2 and 3 are out of order
	headSeq.Append(blocks.Head(3))
	headSeq.Append(blocks.Head(2))

	// Block 4 comes in
	headSeq.Append(blocks.Head(4))

	// Another block at level 4 comes in, that will be uncled
	headSeq.Append(blocks.NewHead(4))

	// Reorg happened forking from block 2
	blocksForked := blocks.ForkAt(t, 2, 5)
	headSeq.Append(blocksForked.Head(2))
	headSeq.Append(blocksForked.Head(3))
	headSeq.Append(blocksForked.Head(4))
	headSeq.Append(blocksForked.Head(5)) // Now the new chain is longer

	lastLongestChainAwaiter := cltest.NewAwaiter()

	// the callback is only called for head number 5 because of head sampling
	checker.On("OnNewLongestChain", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			h := args.Get(1).(*types.Head)
			fmt.Println("OnNewLongestChain", h.BlockNumber())

			assert.Equal(t, int64(5), h.BlockNumber())
			assert.Equal(t, blocksForked.Head(5).BlockHash(), h.BlockHash())

			// This is the new longest chain, check that it came with its parents
			if !assert.NotNil(t, h.Parent) {
				return
			}
			assert.Equal(t, h.Parent.BlockHash(), blocksForked.Head(4).BlockHash())
			if !assert.NotNil(t, h.Parent.Parent) {
				return
			}
			assert.Equal(t, h.Parent.Parent.BlockHash(), blocksForked.Head(3).BlockHash())
			if !assert.NotNil(t, h.Parent.Parent.Parent) {
				return
			}
			assert.Equal(t, h.Parent.Parent.Parent.BlockHash(), blocksForked.Head(2).BlockHash())
			if !assert.NotNil(t, h.Parent.Parent.Parent.Parent) {
				return
			}
			assert.Equal(t, h.Parent.Parent.Parent.Parent.BlockHash(), blocksForked.Head(1).BlockHash())
			lastLongestChainAwaiter.ItHappened()
		}).Return().Once()

	headers := <-chchHeaders

	// This grotesque construction is the only way to do dynamic return values using
	// the mock package.  We need dynamic returns because we're simulating reorgs.
	latestHeadByNumber := make(map[int64]*types.Head)
	latestHeadByNumberMu := new(sync.Mutex)

	fnCall := client.On("HeadByNumber", mock.Anything, mock.Anything)
	fnCall.RunFn = func(args mock.Arguments) {
		latestHeadByNumberMu.Lock()
		defer latestHeadByNumberMu.Unlock()
		num := args.Get(1).(*big.Int)
		head, exists := latestHeadByNumber[num.Int64()]
		if !exists {
			head = cltest.Head(num.Int64())
			latestHeadByNumber[num.Int64()] = head
		}
		fnCall.ReturnArguments = mock.Arguments{head, nil}
	}

	for _, h := range headSeq.Heads {
		latestHeadByNumberMu.Lock()
		latestHeadByNumber[h.BlockNumber()] = h
		latestHeadByNumberMu.Unlock()
		headers.TrySend(h)
	}

	// default 10s may not be sufficient, so using testutils.WaitTimeout(t)
	lastLongestChainAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))
	ht.Stop(t)
	assert.Equal(t, int64(5), ht.headSaver.LatestChain().BlockNumber())

	for _, h := range headSeq.Heads {
		c := ht.headSaver.Chain(h.BlockHash())
		require.NotNil(t, c)
		assert.Equal(t, c.GetParentHash(), h.GetParentHash())
		assert.Equal(t, c.BlockNumber(), h.BlockNumber())
	}
}

// TODO: Fix this test later
func TestHeadTracker_SwitchesToLongestChainWithHeadSamplingDisabled(t *testing.T) {
	t.Parallel()
	lggr, _ := logger.New()
	cfg := headtracker.NewConfig()
	headSaver := headtracker.NewSaver(cfg, lggr)
	client := cltest.NewClientMockWithDefaultChain(t)
	mockChain := &testutils.MockChain{Client: client}
	chchHeaders := make(chan testutils.RawSub[*types.Head], 1)
	checker := commonmocks.NewHeadTrackable[*types.Head, types.Hash](t)
	ht := createHeadTrackerWithChecker(t, cfg, client, headSaver, checker)
	cfg.SetHeadTrackerSamplingInterval(0)
	cfg.SetHeadTrackerMaxBufferSize(100)

	client.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(
			func(ctx context.Context, ch chan<- *types.Head) commontypes.Subscription {
				sub := mockChain.NewSub(t)
				chchHeaders <- testutils.NewRawSub(ch, sub.Err())
				return sub
			},
			func(ctx context.Context, ch chan<- *types.Head) error { return nil },
		)

	// ---------------------
	blocks := cltest.NewBlocks(t, 10)

	head0 := blocks.Head(0) // types.Head{Number: 0, Hash: utils.NewSolanaHash(), ParentHash: utils.NewSolanaHash(), Timestamp: time.Unix(0, 0)}
	// Initial query
	client.On("HeadByNumber", mock.Anything, (*big.Int)(nil)).Return(head0, nil)

	headSeq := cltest.NewHeadBuffer(t)
	headSeq.Append(blocks.Head(0))
	headSeq.Append(blocks.Head(1))

	// Blocks 2 and 3 are out of order
	headSeq.Append(blocks.Head(3))
	headSeq.Append(blocks.Head(2))

	// Block 4 comes in
	headSeq.Append(blocks.Head(4))

	// Another block at level 4 comes in, that will be uncled
	headSeq.Append(blocks.NewHead(4))

	// Reorg happened forking from block 2
	blocksForked := blocks.ForkAt(t, 2, 5)
	headSeq.Append(blocksForked.Head(2))
	headSeq.Append(blocksForked.Head(3))
	headSeq.Append(blocksForked.Head(4))
	headSeq.Append(blocksForked.Head(5)) // Now the new chain is longer

	// --------------------- Delete
	// Print HeadSequence in a nice format
	fmt.Println("headSeq", headSeq.Heads)

	// Iterate over Head sequence, print head.BlockHash() and parent.BlockHash()
	// Add new line after each head
	for _, h := range headSeq.Heads {
		if h.Parent == nil {
			fmt.Printf("head %s, parent nil \n\n", h.BlockHash())
			continue
		}
		fmt.Printf("head hash %s head number %d, parent hash %s parent number %d \n\n ", h.BlockHash(), h.BlockNumber(), h.Parent.BlockHash(), h.Parent.BlockNumber())
	}
	// --------------------- Delete

	lastLongestChainAwaiter := cltest.NewAwaiter()

	checker.On("OnNewLongestChain", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			h := args.Get(1).(*types.Head)
			require.Equal(t, int64(0), h.BlockNumber())
			require.Equal(t, blocks.Head(0).BlockHash(), h.BlockHash())
		}).Return().Once()

	checker.On("OnNewLongestChain", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			h := args.Get(1).(*types.Head)
			require.Equal(t, int64(1), h.BlockNumber())
			require.Equal(t, blocks.Head(1).BlockHash(), h.BlockHash())

			fmt.Println("good h number, hash", h.BlockNumber(), h.BlockHash())

		}).Return().Once()

	checker.On("OnNewLongestChain", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			h := args.Get(1).(*types.Head)
			require.Equal(t, int64(3), h.BlockNumber())
			require.Equal(t, blocks.Head(3).BlockHash(), h.BlockHash())

			// Get parent parent parent
			fmt.Println("problematic h number, hash", h.BlockNumber(), h.BlockHash())

			// Get parent
			fmt.Println("Parent of problematic h", h.Parent)
		}).Return().Once()

	checker.On("OnNewLongestChain", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			h := args.Get(1).(*types.Head)
			require.Equal(t, int64(4), h.BlockNumber())
			require.Equal(t, blocks.Head(4).BlockHash(), h.BlockHash())

			// Check that the block came with its parents
			require.NotNil(t, h.Parent)
			require.Equal(t, h.Parent.BlockHash(), blocks.Head(3).BlockHash())
			require.NotNil(t, h.Parent.Parent.BlockHash()) // 2
			require.Equal(t, h.Parent.Parent.BlockHash(), blocks.Head(2).BlockHash())
			require.NotNil(t, h.Parent.Parent.Parent)
			require.Equal(t, h.Parent.Parent.Parent.BlockHash(), blocks.Head(1).BlockHash())
		}).Return().Once()

	checker.On("OnNewLongestChain", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			h := args.Get(1).(*types.Head)

			require.Equal(t, int64(5), h.BlockNumber())
			require.Equal(t, blocksForked.Head(5).BlockHash(), h.BlockHash())

			// This is the new longest chain, check that it came with its parents
			require.NotNil(t, h.Parent)
			require.Equal(t, h.Parent.BlockHash(), blocksForked.Head(4).BlockHash())
			require.NotNil(t, h.Parent.Parent)
			require.Equal(t, h.Parent.Parent.BlockHash(), blocksForked.Head(3).BlockHash())
			require.NotNil(t, h.Parent.Parent.Parent)
			require.Equal(t, h.Parent.Parent.Parent.BlockHash(), blocksForked.Head(2).BlockHash())
			require.NotNil(t, h.Parent.Parent.Parent.Parent)
			require.Equal(t, h.Parent.Parent.Parent.Parent.BlockHash(), blocksForked.Head(1).BlockHash())
			lastLongestChainAwaiter.ItHappened()
		}).Return().Once()

	ht.Start(t)

	headers := <-chchHeaders

	// This grotesque construction is the only way to do dynamic return values using
	// the mock package.  We need dynamic returns because we're simulating reorgs.
	latestHeadByNumber := make(map[int64]*types.Head)
	latestHeadByNumberMu := new(sync.Mutex)

	fnCall := client.On("HeadByNumber", mock.Anything, mock.Anything)
	fnCall.RunFn = func(args mock.Arguments) {
		latestHeadByNumberMu.Lock()
		defer latestHeadByNumberMu.Unlock()
		num := args.Get(1).(*big.Int)
		head, exists := latestHeadByNumber[num.Int64()]
		if !exists {
			head = cltest.Head(num)
			latestHeadByNumber[num.Int64()] = head
		}
		fnCall.ReturnArguments = mock.Arguments{head, nil}
	}

	for _, h := range headSeq.Heads {
		latestHeadByNumberMu.Lock()
		latestHeadByNumber[h.BlockNumber()] = h
		latestHeadByNumberMu.Unlock()
		headers.TrySend(h)
		time.Sleep(testutils.TestInterval)
	}

	// default 10s may not be sufficient, so using testutils.WaitTimeout(t)
	lastLongestChainAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))
	ht.Stop(t)
	assert.Equal(t, int64(5), ht.headSaver.LatestChain().BlockNumber())

	for _, h := range headSeq.Heads {
		c := ht.headSaver.Chain(h.BlockHash())
		require.NotNil(t, c)
		assert.Equal(t, c.GetParentHash(), h.GetParentHash())
		assert.Equal(t, c.BlockNumber(), h.BlockNumber())
	}
}

func TestHeadTracker_Backfill(t *testing.T) {
	t.Parallel()

	// Heads are arranged as follows:
	// headN indicates an unpersisted ethereum header
	// hN indicates a persisted head record
	//
	// (1)->(H0)
	//
	//       (14Orphaned)-+
	//                    +->(13)->(12)->(11)->(H10)->(9)->(H8)
	// (15)->(14)---------+

	head0 := cltest.Head(0)

	h1 := cltest.Head(1)
	h1.Block.PreviousBlockhash = head0.BlockHash().Hash

	fmt.Println("\n\n head0 Blockhash \n", head0.BlockHash().Hash)
	fmt.Println("\n\n h1 Blockhash \n", h1.GetParentHash())
	fmt.Println("h1 Blockhash \n", h1.Block.PreviousBlockhash)

	h8 := cltest.Head(8)

	h9 := cltest.Head(9)
	h9.Block.PreviousBlockhash = h8.BlockHash().Hash

	h10 := cltest.Head(10)
	h10.Block.PreviousBlockhash = h9.BlockHash().Hash

	h11 := cltest.Head(11)
	h11.Block.PreviousBlockhash = h10.BlockHash().Hash

	h12 := cltest.Head(12)
	h12.Block.PreviousBlockhash = h11.BlockHash().Hash

	h13 := cltest.Head(13)
	h13.Block.PreviousBlockhash = h12.BlockHash().Hash

	h14Orphaned := cltest.Head(14)
	h14Orphaned.Block.PreviousBlockhash = h13.BlockHash().Hash

	h14 := cltest.Head(14)
	h14.Block.PreviousBlockhash = h13.BlockHash().Hash

	h15 := cltest.Head(15)
	h15.Block.PreviousBlockhash = h14.BlockHash().Hash

	heads := []types.Head{
		*h9,
		*h11,
		*h12,
		*h13,
		*h14Orphaned,
		*h14,
		*h15,
	}

	ctx := testutils.Context(t)

	t.Run("does nothing if all the heads are in headsaver", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)

		for i := range heads {
			require.NoError(t, headSaver.Save(testutils.Context(t), &heads[i]))
		}

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)
		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		err := ht.Backfill(ctx, h12, 2)
		require.NoError(t, err)
	})

	t.Run("fetches a missing head", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)

		for i := range heads {
			require.NoError(t, headSaver.Save(testutils.Context(t), &heads[i]))
		}

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)
		client.On("HeadByNumber", mock.Anything, big.NewInt(10)).
			Return(h10, nil)

		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		var depth uint = 3

		// Should backfill h10
		err := ht.Backfill(ctx, h12, depth)
		require.NoError(t, err)

		h := ht.headSaver.Chain(h12.BlockHash())

		assert.Equal(t, int64(12), h.BlockNumber())
		require.NotNil(t, h.Parent)
		assert.Equal(t, int64(11), h.Parent.BlockNumber())
		require.NotNil(t, h.Parent.Parent)
		assert.Equal(t, int64(10), h.Parent.Parent.BlockNumber())
		require.NotNil(t, h.Parent.Parent.Parent)
		assert.Equal(t, int64(9), h.Parent.Parent.Parent.BlockNumber())

		writtenHead, err := headSaver.HeadByHash(h10.BlockHash())
		require.NoError(t, err)
		assert.Equal(t, int64(10), writtenHead.BlockNumber())
	})

	t.Run("fetches only heads that are missing", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)

		for i := range heads {
			require.NoError(t, headSaver.Save(testutils.Context(t), &heads[i]))
		}

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)

		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		client.On("HeadByNumber", mock.Anything, big.NewInt(10)).
			Return(h10, nil)
		client.On("HeadByNumber", mock.Anything, big.NewInt(8)).
			Return(h8, nil)

		// Needs to be 8 because there are 8 heads in chain (15,14,13,12,11,10,9,8)
		var depth uint = 8

		err := ht.Backfill(ctx, h15, depth)
		require.NoError(t, err)

		h := ht.headSaver.Chain(h15.BlockHash())

		require.Equal(t, uint32(8), h.ChainLength())
		earliestInChain := h.EarliestHeadInChain()
		assert.Equal(t, h8.BlockNumber(), earliestInChain.BlockNumber())
		assert.Equal(t, h8.BlockHash(), earliestInChain.BlockHash())
	})

	t.Run("does not backfill if chain length is already greater than or equal to depth", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)

		for i := range heads {
			require.NoError(t, headSaver.Save(testutils.Context(t), &heads[i]))
		}

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)

		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		err := ht.Backfill(ctx, h15, 3)
		require.NoError(t, err)

		err = ht.Backfill(ctx, h15, 5)
		require.NoError(t, err)
	})

	t.Run("only backfills to height 0 if chain length would otherwise cause it to try and fetch a negative head", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)
		client.On("HeadByNumber", mock.Anything, big.NewInt(0)).
			Return(head0, nil)

		require.NoError(t, headSaver.Save(testutils.Context(t), h1))

		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		err := ht.Backfill(ctx, h1, 400)
		require.NoError(t, err)

		h := ht.headSaver.Chain(h1.BlockHash())
		require.NotNil(t, h)

		require.Equal(t, uint32(2), h.ChainLength())
		require.Equal(t, int64(0), h.EarliestHeadInChain().BlockNumber())
	})

	t.Run("abandons backfill and returns error if the eth node returns not found", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)

		for i := range heads {
			require.NoError(t, headSaver.Save(testutils.Context(t), &heads[i]))
		}

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)

		client.On("HeadByNumber", mock.Anything, big.NewInt(10)).
			Return(h10, nil).
			Once()
		client.On("HeadByNumber", mock.Anything, big.NewInt(8)).
			Return(cltest.Head(0), errors.New("not found")).
			Once()

		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		err := ht.Backfill(ctx, h12, 400)
		require.Error(t, err)
		require.EqualError(t, err, "fetchAndSaveHead failed: not found")

		h := ht.headSaver.Chain(h12.BlockHash())

		// Should contain 12, 11, 10, 9
		assert.Equal(t, 4, int(h.ChainLength()))
		assert.Equal(t, int64(9), h.EarliestHeadInChain().BlockNumber())
	})

	t.Run("abandons backfill and returns error if the context time budget is exceeded", func(t *testing.T) {
		lggr, _ := logger.New()
		cfg := headtracker.NewConfig()
		headSaver := headtracker.NewSaver(cfg, lggr)
		for i := range heads {
			require.NoError(t, headSaver.Save(testutils.Context(t), &heads[i]))
		}

		client := cltest.NewClientMock(t)
		client.On("ConfiguredChainID", mock.Anything).Return(types.Localnet, nil)
		client.On("HeadByNumber", mock.Anything, big.NewInt(10)).
			Return(h10, nil)
		client.On("HeadByNumber", mock.Anything, big.NewInt(8)).
			Return(cltest.Head(0), context.DeadlineExceeded)

		ht := createHeadTrackerWithNeverSleeper(t, cfg, client, headSaver)

		err := ht.Backfill(ctx, h12, 400)
		require.Error(t, err)
		require.EqualError(t, err, "fetchAndSaveHead failed: context deadline exceeded")

		h := ht.headSaver.Chain(h12.BlockHash())

		// Should contain 12, 11, 10, 9
		assert.Equal(t, 4, int(h.ChainLength()))
		assert.Equal(t, int64(9), h.EarliestHeadInChain().BlockNumber())
	})
}

// Helper Functions

func createHeadTracker(
	t *testing.T,
	config *headtracker.Config,
	solanaClient htrktypes.Client[
		*types.Head,
		commontypes.Subscription,
		types.ChainID,
		types.Hash],
	hs *headtracker.InMemoryHeadSaver[
		*types.Head,
		types.Hash,
		types.ChainID],
) *headTrackerUniverse {
	lggr, _ := logger.New()
	hb := headtracker.NewBroadcaster(lggr)
	mailMon := utils.NewMailboxMonitor(t.Name())
	ht := headtracker.NewTracker(lggr, solanaClient, config, hb, hs, mailMon)
	return &headTrackerUniverse{
		mu:              new(sync.Mutex),
		headTracker:     ht,
		headBroadcaster: hb,
		headSaver:       hs,
		mailMon:         mailMon,
	}
}

func createHeadTrackerWithNeverSleeper(t *testing.T,
	config *headtracker.Config,
	solanaClient htrktypes.Client[
		*types.Head,
		commontypes.Subscription,
		types.ChainID,
		types.Hash],
	hs *headtracker.InMemoryHeadSaver[
		*types.Head,
		types.Hash,
		types.ChainID]) *headTrackerUniverse {
	lggr, _ := logger.New()
	hb := headtracker.NewBroadcaster(lggr)
	mailMon := utils.NewMailboxMonitor(t.Name())
	ht := headtracker.NewTracker(lggr, solanaClient, config, hb, hs, mailMon)
	return &headTrackerUniverse{
		mu:              new(sync.Mutex),
		headTracker:     ht,
		headBroadcaster: hb,
		headSaver:       hs,
		mailMon:         mailMon,
	}
}

func createHeadTrackerWithChecker(t *testing.T,
	config *headtracker.Config,
	solanaClient htrktypes.Client[
		*types.Head,
		commontypes.Subscription,
		types.ChainID,
		types.Hash],
	hs *headtracker.InMemoryHeadSaver[
		*types.Head,
		types.Hash,
		types.ChainID],
	checker commontypes.HeadTrackable[*types.Head, types.Hash],
) *headTrackerUniverse {
	lggr, _ := logger.New()
	hb := headtracker.NewBroadcaster(lggr)

	hb.Subscribe(checker)
	mailMon := utils.NewMailboxMonitor(t.Name())
	ht := headtracker.NewTracker(lggr, solanaClient, config, hb, hs, mailMon)
	return &headTrackerUniverse{
		mu:              new(sync.Mutex),
		headTracker:     ht,
		headBroadcaster: hb,
		headSaver:       hs,
		mailMon:         mailMon,
	}
}

type headTrackerUniverse struct {
	mu              *sync.Mutex
	stopped         bool
	headTracker     commontypes.HeadTracker[*types.Head, types.Hash]
	headBroadcaster commontypes.HeadBroadcaster[*types.Head, types.Hash]
	headSaver       commontypes.HeadSaver[*types.Head, types.Hash]
	mailMon         *utils.MailboxMonitor
}

func (u *headTrackerUniverse) Backfill(ctx context.Context, head *types.Head, depth uint) error {
	return u.headTracker.Backfill(ctx, head, depth)
}

func (u *headTrackerUniverse) Start(t *testing.T) {
	u.mu.Lock()
	defer u.mu.Unlock()
	ctx := testutils.Context(t)
	require.NoError(t, u.headBroadcaster.Start(ctx))
	require.NoError(t, u.headTracker.Start(ctx))
	require.NoError(t, u.mailMon.Start(ctx))

	g := gomega.NewWithT(t)
	g.Eventually(func() bool {
		report := u.headBroadcaster.HealthReport()
		return !slices.ContainsFunc(maps.Values(report), func(e error) bool { return e != nil })
	}, 5*time.Second, testutils.TestInterval).Should(gomega.Equal(true))

	t.Cleanup(func() {
		u.Stop(t)
	})
}

func (u *headTrackerUniverse) Stop(t *testing.T) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.stopped {
		return
	}
	u.stopped = true
	require.NoError(t, u.headBroadcaster.Close())
	require.NoError(t, u.headTracker.Close())
	require.NoError(t, u.mailMon.Close())
}

func ptr[T any](t T) *T { return &t }
