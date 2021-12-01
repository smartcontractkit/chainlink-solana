package solana

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/stretchr/testify/assert"
)

func TestLatestBlockHeight(t *testing.T) {
	ctx := context.Background()
	c := &ContractTracker{
		client: NewClient(rpc.DevNet_RPC, &ws.Client{}),
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
