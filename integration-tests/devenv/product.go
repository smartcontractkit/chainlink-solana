package devenv

import (
	"context"

	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products/solana"
)

type ProductInfo struct {
	Name      string `toml:"name"`
	Instances int    `toml:"instances"`
}

type Cfg struct {
	Products []*ProductInfo      `toml:"products"`
	Solana   *solana.SolanaInput `toml:"solana"`
	NodeSets []*ns.Input         `toml:"nodesets" validate:"required"`
}

// Product describes a minimal set of methods that each product must implement.
// Mirrors devenv.Product but with Solana-specific infra types.
type Product interface {
	Load() error
	Store(path string, instanceIdx int) error
	GenerateNodesConfig(ctx context.Context, sol *solana.SolanaInput, ns []*ns.Input) (string, error)
	GenerateNodesSecrets(ctx context.Context, sol *solana.SolanaInput, ns []*ns.Input) (string, error)
	ConfigureJobsAndContracts(ctx context.Context, instanceIdx int, sol *solana.SolanaInput, fakesURL string, ns []*ns.Input) error
}
