package devenv

import (
	"context"

	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"
)

// Product describes a minimal set of methods that each product must implement.
// Mirrors devenv.Product but with Solana-specific infra types.
type Product interface {
	Load() error
	Store(path string, instanceIdx int) error
	GenerateNodesConfig(ctx context.Context, sol *SolanaInput, parrot *ParrotInput, ns []*ns.Input) (string, error)
	GenerateNodesSecrets(ctx context.Context, sol *SolanaInput, parrot *ParrotInput, ns []*ns.Input) (string, error)
	ConfigureJobsAndContracts(ctx context.Context, instanceIdx int, sol *SolanaInput, parrot *ParrotInput, ns []*ns.Input) error
}
