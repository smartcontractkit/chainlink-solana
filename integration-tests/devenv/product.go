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

// Cfg is the top-level environment config, mirroring devenv.Cfg
// but with Solana-specific infrastructure instead of EVM blockchain.
type Cfg struct {
	Products []*ProductInfo      `toml:"products"`
	Solana   *solana.SolanaInput `toml:"solana"`
	Parrot   *solana.ParrotInput `toml:"parrot"`
	NodeSets []*ns.Input         `toml:"nodesets" validate:"required"`
}

// Product describes a minimal set of methods that each product must implement.
// Mirrors devenv.Product but with Solana-specific infra types.
type Product interface {
	Load() error
	Store(path string, instanceIdx int) error
	GenerateNodesConfig(ctx context.Context, sol *solana.SolanaInput, parrot *solana.ParrotInput, ns []*ns.Input) (string, error)
	GenerateNodesSecrets(ctx context.Context, sol *solana.SolanaInput, parrot *solana.ParrotInput, ns []*ns.Input) (string, error)
	ConfigureJobsAndContracts(ctx context.Context, instanceIdx int, sol *solana.SolanaInput, parrot *solana.ParrotInput, ns []*ns.Input) error
}
