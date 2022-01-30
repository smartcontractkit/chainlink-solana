package solana

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"

	relay "github.com/smartcontractkit/chainlink-relay/pkg/plugin"
	"github.com/smartcontractkit/chainlink/core/logger"
)

func TestLatestBlockHeight(t *testing.T) {
	ctx := context.Background()
	c := &ContractTracker{
		client: NewClient(relay.SolanaSpec{NodeEndpointHTTP: rpc.DevNet_RPC}, logger.TestLogger(t)),
	}

	h, err := c.LatestBlockHeight(ctx)
	assert.NoError(t, err)
	assert.True(t, h > 0)
}
