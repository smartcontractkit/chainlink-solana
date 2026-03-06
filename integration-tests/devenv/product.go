package devenv

import (
	"context"

	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/components/solana"
)

type ProductInfo struct {
	Name      string `toml:"name" validate:"required"`
	Instances int    `toml:"instances" validate:"required"`
}

type Cfg struct {
	Products []*ProductInfo `toml:"products" validate:"required"`
	Solana   *solana.Input  `toml:"solana" validate:"required"`
	NodeSets []*ns.Input    `toml:"nodesets" validate:"required"`
}

// Product describes a minimal set of methods that each product must implement.
// Mirrors devenv.Product but with Solana-specific infra types.
type Product interface {
	Load() error
	Store(path string, instanceIdx int) error
	GenerateNodesConfig(ctx context.Context, sol *solana.Input, ns []*ns.Input) (string, error)
	GenerateNodesSecrets(ctx context.Context, sol *solana.Input, ns []*ns.Input) (string, error)
	ConfigureJobsAndContracts(ctx context.Context, instanceIdx int, sol *solana.Input, fakesURL string, ns []*ns.Input) error
}
