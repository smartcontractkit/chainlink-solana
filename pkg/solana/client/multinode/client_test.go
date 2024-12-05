package client

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode/config"
)

type TestRPC struct {
	latestBlock int64
}

type TestHead struct {
	blockNumber int64
}

func (t *TestHead) BlockNumber() int64        { return t.blockNumber }
func (t *TestHead) BlockDifficulty() *big.Int { return nil }
func (t *TestHead) IsValid() bool             { return true }

func LatestBlock(ctx context.Context, rpc *TestRPC) (*TestHead, error) {
	rpc.latestBlock++
	return &TestHead{rpc.latestBlock}, nil
}

func initializeMultiNodeClient(t *testing.T) *MultiNodeClient[TestRPC, *TestHead] {
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := &config.MultiNodeConfig{}
	cfg.SetDefaults()
	enabled := true
	cfg.MultiNode.Enabled = &enabled

	c, err := NewMultiNodeClient[TestRPC, *TestHead](cfg, &TestRPC{}, requestTimeout, lggr, LatestBlock, LatestBlock)
	require.NoError(t, err)
	return c
}

func TestMultiNodeClient_LatestBlock(t *testing.T) {
	c := initializeMultiNodeClient(t)

	t.Run("LatestBlock", func(t *testing.T) {
		head, err := c.LatestBlock(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, true, head.IsValid())
	})

	t.Run("LatestFinalizedBlock", func(t *testing.T) {
		finalizedHead, err := c.LatestFinalizedBlock(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, true, finalizedHead.IsValid())
	})
}

func TestMultiNodeClient_HeadSubscriptions(t *testing.T) {
	c := initializeMultiNodeClient(t)

	t.Run("SubscribeToHeads", func(t *testing.T) {
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
	c := initializeMultiNodeClient(t)

	t.Run("registerSub", func(t *testing.T) {
		sub := newMockSub()
		err := c.registerSub(sub, make(chan struct{}))
		require.NoError(t, err)
		require.Equal(t, 1, c.LenSubs())
		c.UnsubscribeAllExcept()
	})

	t.Run("chStopInFlight returns error and unsubscribes", func(t *testing.T) {
		chStopInFlight := make(chan struct{})
		close(chStopInFlight)
		sub := newMockSub()
		err := c.registerSub(sub, chStopInFlight)
		require.Error(t, err)
		require.Equal(t, true, sub.unsubscribed)
	})

	t.Run("UnsubscribeAllExcept", func(t *testing.T) {
		chStopInFlight := make(chan struct{})
		sub1 := newMockSub()
		sub2 := newMockSub()
		err := c.registerSub(sub1, chStopInFlight)
		require.NoError(t, err)
		err = c.registerSub(sub2, chStopInFlight)
		require.NoError(t, err)
		require.Equal(t, 2, c.LenSubs())

		c.UnsubscribeAllExcept(sub1)
		require.Equal(t, 1, c.LenSubs())
		require.Equal(t, true, sub2.unsubscribed)

		c.UnsubscribeAllExcept()
		require.Equal(t, 0, c.LenSubs())
		require.Equal(t, true, sub1.unsubscribed)
	})
}
