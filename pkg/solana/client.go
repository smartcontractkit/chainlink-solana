package solana

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/sync/singleflight"
)

// Client contains the rpc and requestGroup for a given network
type Client struct {
	rpc           *rpc.Client
	skipPreflight bool // to enable or disable preflight checks
	commitment    rpc.CommitmentType

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

// NewClient will bundle the RPC and requestGroup together as a network Client
func NewClient(rpcEndpoint string, skipPreflight bool, commitment string) *Client {
	client := &Client{
		rpc:           rpc.New(rpcEndpoint),
		skipPreflight: skipPreflight,
		requestGroup:  &singleflight.Group{},
	}

	switch commitment {
	case "processed":
		client.commitment = rpc.CommitmentProcessed
	case "finalized":
		client.commitment = rpc.CommitmentProcessed
	default:
		client.commitment = rpc.CommitmentConfirmed
	}

	return client
}

// GetBlockHeight returns the height of the most recent processed block in the chain, coalescing requests.
func (c Client) GetBlockHeight(ctx context.Context, commitment rpc.CommitmentType) (blockHeight uint64, err error) {
	// do single flight request
	v, err, _ := c.requestGroup.Do("GetSlotHeight", func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, commitment)
	})

	if err != nil {
		return 0, err
	}

	return v.(uint64), nil
}
