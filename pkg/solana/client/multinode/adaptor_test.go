package client

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	common "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode/config"
)

type testRPC struct {
	latestBlock int64
}

type testHead struct {
	blockNumber int64
}

func (t *testHead) BlockNumber() int64        { return t.blockNumber }
func (t *testHead) BlockDifficulty() *big.Int { return nil }
func (t *testHead) IsValid() bool             { return true }

func LatestBlock(ctx context.Context, rpc *testRPC) (*testHead, error) {
	rpc.latestBlock++
	return &testHead{rpc.latestBlock}, nil
}

func ptr[T any](t T) *T {
	return &t
}

func newTestClient(t *testing.T) *MultiNodeAdapter[testRPC, *testHead] {
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := &config.MultiNodeConfig{
		MultiNode: config.MultiNode{
			Enabled:                      ptr(true),
			PollFailureThreshold:         ptr(uint32(5)),
			PollInterval:                 common.MustNewDuration(15 * time.Second),
			SelectionMode:                ptr(NodeSelectionModePriorityLevel),
			SyncThreshold:                ptr(uint32(10)),
			LeaseDuration:                common.MustNewDuration(time.Minute),
			NodeIsSyncingEnabled:         ptr(false),
			FinalizedBlockPollInterval:   common.MustNewDuration(5 * time.Second),
			EnforceRepeatableRead:        ptr(true),
			DeathDeclarationDelay:        common.MustNewDuration(20 * time.Second),
			NodeNoNewHeadsThreshold:      common.MustNewDuration(20 * time.Second),
			NoNewFinalizedHeadsThreshold: common.MustNewDuration(20 * time.Second),
			FinalityTagEnabled:           ptr(true),
			FinalityDepth:                ptr(uint32(0)),
			FinalizedBlockOffset:         ptr(uint32(50)),
		},
	}
	c, err := NewMultiNodeAdapter[testRPC, *testHead](cfg, &testRPC{}, requestTimeout, lggr, LatestBlock, LatestBlock)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}

func TestMultiNodeClient_LatestBlock(t *testing.T) {
	t.Run("LatestBlock", func(t *testing.T) {
		c := newTestClient(t)
		head, err := c.LatestBlock(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, true, head.IsValid())
	})

	t.Run("LatestFinalizedBlock", func(t *testing.T) {
		c := newTestClient(t)
		finalizedHead, err := c.LatestFinalizedBlock(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, true, finalizedHead.IsValid())
	})
}

func TestMultiNodeClient_HeadSubscriptions(t *testing.T) {
	t.Run("SubscribeToHeads", func(t *testing.T) {
		c := newTestClient(t)
		ch, sub, err := c.SubscribeToHeads(tests.Context(t))
		require.NoError(t, err)
		defer sub.Unsubscribe()

		ctx, cancel := context.WithTimeout(tests.Context(t), time.Minute)
		defer cancel()
		select {
		case head := <-ch:
			latest, _ := c.GetInterceptedChainInfo()
			require.Equal(t, head.BlockNumber(), latest.BlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive head: ", ctx.Err())
		}
	})

	t.Run("SubscribeToFinalizedHeads", func(t *testing.T) {
		c := newTestClient(t)
		finalizedCh, finalizedSub, err := c.SubscribeToFinalizedHeads(tests.Context(t))
		require.NoError(t, err)
		defer finalizedSub.Unsubscribe()

		ctx, cancel := context.WithTimeout(tests.Context(t), time.Minute)
		defer cancel()
		select {
		case finalizedHead := <-finalizedCh:
			latest, _ := c.GetInterceptedChainInfo()
			require.Equal(t, finalizedHead.BlockNumber(), latest.FinalizedBlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive finalized head: ", ctx.Err())
		}
	})

	t.Run("Remove Subscription on Unsubscribe", func(t *testing.T) {
		c := newTestClient(t)
		_, sub1, err := c.SubscribeToHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 1, c.LenSubs())
		_, sub2, err := c.SubscribeToFinalizedHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 2, c.LenSubs())

		sub1.Unsubscribe()
		require.Equal(t, 1, c.LenSubs())
		sub2.Unsubscribe()
		require.Equal(t, 0, c.LenSubs())
	})

	t.Run("Ensure no deadlock on UnsubscribeAll", func(t *testing.T) {
		c := newTestClient(t)
		_, _, err := c.SubscribeToHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 1, c.LenSubs())
		_, _, err = c.SubscribeToFinalizedHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 2, c.LenSubs())
		c.UnsubscribeAllExcept()
		require.Equal(t, 0, c.LenSubs())
	})
}

type mockSub struct {
	unsubscribed bool
}

func newMockSub() *mockSub {
	return &mockSub{unsubscribed: false}
}

func (s *mockSub) Unsubscribe() {
	s.unsubscribed = true
}
func (s *mockSub) Err() <-chan error {
	return nil
}

func TestMultiNodeClient_RegisterSubs(t *testing.T) {
	t.Run("registerSub", func(t *testing.T) {
		c := newTestClient(t)
		mockSub := newMockSub()
		sub := &ManagedSubscription{
			Subscription:  mockSub,
			onUnsubscribe: c.removeSub,
		}
		err := c.registerSub(sub, make(chan struct{}))
		require.NoError(t, err)
		require.Equal(t, 1, c.LenSubs())
		c.UnsubscribeAllExcept()
		require.Equal(t, 0, c.LenSubs())
		require.Equal(t, true, mockSub.unsubscribed)
	})

	t.Run("chStopInFlight returns error and unsubscribes", func(t *testing.T) {
		c := newTestClient(t)
		chStopInFlight := make(chan struct{})
		close(chStopInFlight)
		mockSub := newMockSub()
		sub := &ManagedSubscription{
			Subscription:  mockSub,
			onUnsubscribe: c.removeSub,
		}
		err := c.registerSub(sub, chStopInFlight)
		require.Error(t, err)
		require.Equal(t, true, mockSub.unsubscribed)
	})

	t.Run("UnsubscribeAllExcept", func(t *testing.T) {
		c := newTestClient(t)
		chStopInFlight := make(chan struct{})
		mockSub1 := newMockSub()
		sub1 := &ManagedSubscription{
			Subscription:  mockSub1,
			onUnsubscribe: c.removeSub,
		}
		mockSub2 := newMockSub()
		sub2 := &ManagedSubscription{
			Subscription:  mockSub2,
			onUnsubscribe: c.removeSub,
		}
		err := c.registerSub(sub1, chStopInFlight)
		require.NoError(t, err)
		err = c.registerSub(sub2, chStopInFlight)
		require.NoError(t, err)
		require.Equal(t, 2, c.LenSubs())

		// Ensure passed sub is not removed
		c.UnsubscribeAllExcept(sub1)
		require.Equal(t, 1, c.LenSubs())
		require.Equal(t, true, mockSub2.unsubscribed)
		require.Equal(t, false, mockSub1.unsubscribed)

		c.UnsubscribeAllExcept()
		require.Equal(t, 0, c.LenSubs())
		require.Equal(t, true, mockSub1.unsubscribed)
	})
}
