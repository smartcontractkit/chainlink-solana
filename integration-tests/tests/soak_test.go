package tests

import (
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products/solana"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/leak"
	"github.com/stretchr/testify/require"
)

func TestSolanaOCRV2Soak(t *testing.T) {
	start := time.Now()

	pdConfig, err := products.LoadOutput[solana.Configurator](defaultEnvOutPath)
	require.NoError(t, err, "Failed to load product config from env-out.toml")
	require.NotEmpty(t, pdConfig.Config, "No OCR2 Solana config found in env-out.toml")

	cfg := pdConfig.Config[0]
	sg, err := gauntlet.NewSolanaGauntlet(cfg.GauntletPath)
	require.NoError(t, err, "Failed to reconstruct gauntlet from saved path")
	sg.G.Network = cfg.GauntletNetwork
	sg.G.Command = "gauntlet-nobuild"

	validateRounds(t, cfg.OcrAddress, sg, cfg.NumberOfRounds)

	l, err := leak.NewCLNodesLeakDetector(leak.NewResourceLeakChecker())
	require.NoError(t, err)
	errs := l.Check(&leak.CLNodesCheck{
		ComparisonMode:  leak.ComparisonModeAbsolute,
		NumNodes:        cfg.NodeCount,
		Start:           start,
		End:             time.Now(),
		WarmUpDuration:  30 * time.Minute,
		CPUThreshold:    25.0,
		MemoryThreshold: 210.0,
	})
	require.NoError(t, errs)
}
