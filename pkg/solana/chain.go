package solana

import (
	"context"

	"github.com/smartcontractkit/chainlink/core/services"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
)

type ChainSet interface {
	services.ServiceCtx
	// Chain returns chain for the given id.
	Chain(ctx context.Context, id string) (Chain, error)
}

type Chain interface {
	services.ServiceCtx

	ID() string
	Config() config.Config
	UpdateConfig(*db.ChainCfg)
	TxManager() TxManager
	// Reader returns a new Reader from the available list of nodes (if there are multiple, it will randomly select one)
	Reader() (client.Reader, error)
}
