package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	solcomp "github.com/smartcontractkit/chainlink-solana/integration-tests/components/solana"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products/solana"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
)

func TestSolanaOCRV2UpgradeSmoke(t *testing.T) {
	pdConfig, err := products.LoadOutput[solana.Configurator](defaultEnvOutPath)
	require.NoError(t, err, "Failed to load product config from env-out.toml")
	require.NotEmpty(t, pdConfig.Config, "No OCR2 Solana config found in env-out.toml")

	infraCfg, err := devenv.LoadOutput[devenv.Cfg](defaultEnvOutPath)
	require.NoError(t, err, "Failed to load infra config from env-out.toml")

	cfg := pdConfig.Config[0]
	sg, err := gauntlet.NewSolanaGauntlet(cfg.GauntletPath)
	require.NoError(t, err, "Failed to reconstruct gauntlet from saved path")
	sg.G.Network = cfg.GauntletNetwork
	sg.G.Command = "gauntlet-nobuild"

	validateRounds(t, cfg.OcrAddress, sg, cfg.NumberOfRounds)

	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|           REDEPLOYING CONTRACTS           |")
	log.Info().Msg("---------------------------------------------")

	upgradeContracts(t, infraCfg, cfg)

	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|                                           |")
	log.Info().Msg("---------------------------------------------")

	validateRounds(t, cfg.OcrAddress, sg, cfg.NumberOfRounds)
}

func upgradeContracts(t *testing.T, infraCfg *devenv.Cfg, cfg *solana.OCR2Solana) {
	t.Helper()

	upgradeDir := os.Getenv("UPGRADE_CONTRACTS_DIR")
	require.NotEmpty(t, upgradeDir, "UPGRADE_CONTRACTS_DIR env var must be set for the upgrade test")

	ctx := context.Background()
	containerName := infraCfg.Solana.Out.ContainerName
	require.NotEmpty(t, containerName, "Solana container name not found in env-out.toml")

	solOut, err := solcomp.FindSolanaByName(ctx, containerName)
	require.NoError(t, err, "Failed to find Solana container by name")

	solClient := &solclient.Client{}
	solClient.Config = solClient.Config.Default()
	solClient.Config.URLs = []string{solOut.ExternalHTTPURL, solOut.ExternalWsURL}
	solClient, err = solclient.NewClient(solClient.Config)
	require.NoError(t, err, "Failed to create Solana client")

	cd, err := solclient.NewContractDeployer(solClient, nil)
	require.NoError(t, err, "Failed to create contract deployer")

	programIDBuilder := func(programName string) string {
		programName, _ = strings.CutSuffix(filepath.Base(programName), ".so")
		ids := map[string]string{
			"ocr_2":             cfg.ProgramAddresses.OCR2,
			"access_controller": cfg.ProgramAddresses.AccessController,
			"store":             cfg.ProgramAddresses.Store,
		}
		val, ok := ids[programName]
		if !ok {
			val = solclient.BuildProgramIDKeypairPath(programName)
			log.Warn().Str("Program", programName).Msg(fmt.Sprintf("falling back to path (%s)", val))
		}
		return val
	}

	err = cd.DeployAnchorProgramsRemoteDocker(upgradeDir, "", solOut.Container, programIDBuilder)
	require.NoError(t, err, "Failed to upgrade contracts")
}
