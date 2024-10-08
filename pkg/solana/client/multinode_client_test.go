package client

import (
	"context"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func initializeMultiNodeClient(t *testing.T) *MultiNodeClient {
	url := SetupLocalSolNode(t)
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	FundTestAccounts(t, []solana.PublicKey{pubKey}, url)

	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	enabled := true
	cfg.MultiNode.MultiNode.Enabled = &enabled

	c, err := NewMultiNodeClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)
	return c
}

func TestMultiNodeClient_Ping(t *testing.T) {
	c := initializeMultiNodeClient(t)
	require.NoError(t, c.Ping(tests.Context(t)))
}

func TestMultiNodeClient_LatestBlock(t *testing.T) {
	c := initializeMultiNodeClient(t)

	t.Run("LatestBlock", func(t *testing.T) {
		head, err := c.LatestBlock(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, true, head.IsValid())
		require.NotEqual(t, solana.Hash{}, head.BlockHash)
	})

	t.Run("LatestFinalizedBlock", func(t *testing.T) {
		finalizedHead, err := c.LatestFinalizedBlock(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, true, finalizedHead.IsValid())
		require.NotEqual(t, solana.Hash{}, finalizedHead.BlockHash)
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
			require.NotEqual(t, solana.Hash{}, head.BlockHash)
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
			require.NotEqual(t, solana.Hash{}, finalizedHead.BlockHash)
			latest, _ := c.GetInterceptedChainInfo()
			require.Equal(t, finalizedHead.BlockNumber(), latest.FinalizedBlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive finalized head: ", ctx.Err())
		}
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
		require.Len(t, c.subs, 1)
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
		require.Len(t, c.subs, 2)

		c.UnsubscribeAllExcept(sub1)
		require.Len(t, c.subs, 1)
		require.Equal(t, true, sub2.unsubscribed)

		c.UnsubscribeAllExcept()
		require.Len(t, c.subs, 0)
		require.Equal(t, true, sub1.unsubscribed)
	})
}
