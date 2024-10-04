package client

import (
	"context"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestMultiNodeClient_Subscriptions_Integration(t *testing.T) {
	url := SetupLocalSolNode(t)
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	FundTestAccounts(t, []solana.PublicKey{pubKey}, url)

	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	// Enable MultiNode
	enabled := true
	cfg.MultiNode.SetDefaults()
	cfg.Enabled = &enabled

	c, err := NewMultiNodeClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	err = c.Ping(tests.Context(t))
	require.NoError(t, err)

	ch, sub, err := c.SubscribeToHeads(tests.Context(t))
	require.NoError(t, err)
	defer sub.Unsubscribe()

	finalizedCh, finalizedSub, err := c.SubscribeToFinalizedHeads(tests.Context(t))
	require.NoError(t, err)
	defer finalizedSub.Unsubscribe()

	require.NoError(t, err)
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

	select {
	case finalizedHead := <-finalizedCh:
		require.NotEqual(t, solana.Hash{}, finalizedHead.BlockHash)
		latest, _ := c.GetInterceptedChainInfo()
		require.Equal(t, finalizedHead.BlockNumber(), latest.FinalizedBlockNumber)
	case <-ctx.Done():
		t.Fatal("failed to receive finalized head: ", ctx.Err())
	}
}
