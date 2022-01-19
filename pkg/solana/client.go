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

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

// NewClient will bundle the RPC and requestGroup together as a network Client
func NewClient(rpcEndpoint string) *Client {
	return &Client{
		rpc:           rpc.New(rpcEndpoint),
		skipPreflight: false,
		requestGroup:  &singleflight.Group{},
	}
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
