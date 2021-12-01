package solana

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"

	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2/types"
)

// Client implements the Blockchain interface and contains other needed pieces
type Client struct {
	rpc *rpc.Client
	ws  *ws.Client
}

func NewConnectedClient(ctx context.Context, rpcEndpoint string, wsEndpoint string) (*Client, error) {
	ws, err := ws.Connect(ctx, wsEndpoint)
	if err != nil {
		return &Client{}, err
	}
	return &Client{
		rpc: rpc.New(rpcEndpoint),
		ws:  ws,
	}, nil
}

// Close should stop any running processes or close open channels, connections, etc
func (c Client) Close() {
	c.ws.Close()
}

func (c Client) OCR() ocrtypes.OnchainKeyring {
	return &OnchainKeyring{}
}

func (c *Client) NewContractTracker(address, jobID string) (*ContractTracker, error) {
	return NewTracker(address, jobID, c)
}
