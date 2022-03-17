package solana

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
)

func TestLatestBlockHeight(t *testing.T) {
	ctx := context.Background()
	c := &ContractTracker{
		reader: testSetupReader(t, rpc.DevNet_RPC),
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
