package solana

import (
	"context"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type ChainSet interface {
	ServiceCtx
	// Chain returns chain for the given id.
	Chain(ctx context.Context, id string) (Chain, error)
}

type Chain interface {
	ServiceCtx

	ID() string
	Config() config.Config
	TxManager() TxManager
	// Reader returns a new Reader from the available list of nodes (if there are multiple, it will randomly select one)
	Reader() (client.Reader, error)
}

// ServiceCtx replaces Service interface due to new Start(ctx) method signature.
type ServiceCtx interface {
	// Start starts the service, context can be cancelled to abort Start routine.
	Start(context.Context) error
	Close() error
	Ready() error
	Healthy() error
}
