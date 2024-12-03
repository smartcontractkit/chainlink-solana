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
