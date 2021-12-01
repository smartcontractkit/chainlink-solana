package solana

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
)

func TestLatestBlockHeight(t *testing.T) {
	ctx := context.Background()
	client, err := NewConnectedClient(ctx, rpc.DevNet_RPC, rpc.DevNet_WS)
	assert.NoError(t, err)

	c := &ContractTracker{
		client: client,
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
