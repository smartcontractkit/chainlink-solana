package solana

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"golang.org/x/sync/singleflight"
)

// Client contains the rpc and websocket connections for a given network
type Client struct {
	rpc *rpc.Client
	ws  *ws.Client

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

// NewClient will bundle the RPC and provided WS client together as a network Client
func NewClient(rpcEndpoint string, wsClient *ws.Client) *Client {
	return &Client{
		rpc:          rpc.New(rpcEndpoint),
		ws:           wsClient,
		requestGroup: &singleflight.Group{},
	}
}

// Connections stores the websocket client to be reused in other jobs
// TODO: handle connection errors & retries
type Connections map[string]*ws.Client

// NewConnectedClient will create and bundle the RPC and WS clients together as a network Client
// It will also check against existing WS connections and reuse or create connections
func (cc Connections) NewConnectedClient(ctx context.Context, rpcEndpoint string, wsEndpoint string) (*Client, error) {
	c, err := cc.GetOrConnect(ctx, wsEndpoint)
	if err != nil {
		return nil, err
	}

	return NewClient(rpcEndpoint, c), nil
}

// GetOrConnect reuses a websocket connection if available, or attempts to connect a new client
func (cc Connections) GetOrConnect(ctx context.Context, url string) (*ws.Client, error) {
	if _, ok := cc[url]; !ok {
		c, err := ws.Connect(ctx, url)
		if err != nil {
			return &ws.Client{}, err
		}
		cc[url] = c
	}
	return cc[url], nil
}

// Close closes all websocket connections
func (cc Connections) Close() error {
	for k, c := range cc {
		c.Close()
		delete(cc, k)
	}
	return nil
}

// TODO: We don't currently share full Client/s, but only the ws.Client/s, so every provider (w/ client) will be in a separate group (no coalescing)
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
