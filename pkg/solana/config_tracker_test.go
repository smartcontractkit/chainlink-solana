package solana

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/assert"
)

func TestLatestBlockHeight(t *testing.T) {
	ctx := context.Background()
	c := &ContractTracker{
		client: NewClient(OCR2Spec{NodeEndpointHTTP: rpc.DevNet_RPC}, logger.TestLogger(t)),
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
