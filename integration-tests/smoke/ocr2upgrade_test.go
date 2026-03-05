package smoke

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	ocr_config "github.com/smartcontractkit/chainlink-solana/integration-tests/config"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	tc "github.com/smartcontractkit/chainlink-solana/integration-tests/testconfig"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

func TestSolanaOCRV2UpgradeSmoke(t *testing.T) {
	name := "plugins-program-upgrade"
	env := map[string]string{
		"CL_MEDIAN_CMD": "chainlink-feeds",
		"CL_SOLANA_CMD": "chainlink-solana",
	}
	config, err := tc.GetConfig("Smoke", tc.OCR2)
	if err != nil {
		t.Fatal(err)
	}

	envOutPath := filepath.Join(os.TempDir(), fmt.Sprintf("sol-ocr2-%s-%s", name, devenv.DefaultEnvOutFile))
	state, sg := setupOCR2UpgradeEnvironment(t, name, env, config, "previous", envOutPath)
	envOut := &devenv.EnvOutput{
		OcrAddress:     sg.OcrAddress,
		FeedAddress:    sg.FeedAddress,
		RPCURLExternal: state.Common.ChainDetails.RPCURLExternal,
		WSURLExternal:  state.Common.ChainDetails.WSURLExternal,
	}

	validateRoundsFromEnv(t, name, envOut, sg, *config.OCR2.NumberOfRounds)

	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|           REDEPLOYING CONTRACTS           |")
	log.Info().Msg("---------------------------------------------")
	state.UpgradeContracts(utils.ContractsDir, "")
	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|                                           |")
	log.Info().Msg("---------------------------------------------")

	validateRoundsFromEnv(t, name, envOut, sg, *config.OCR2.NumberOfRounds)
}

// setupOCR2UpgradeEnvironment sets up OCR2 and returns the state for
// subsequent contract upgrades. Unlike setupOCR2Environment, it returns the
// full state so the caller can call UpgradeContracts.
func setupOCR2UpgradeEnvironment(t *testing.T, testname string, testenv map[string]string, config tc.TestConfig, subDir, envOutPath string) (*common.OCRv2TestState, *gauntlet.SolanaGauntlet) {
	name := "gauntlet-" + testname
	state, err := common.NewOCRv2State(t, 1, name, &config)
	require.NoError(t, err, "Could not setup the ocrv2 state")
	if len(testenv) > 0 {
		if state.Common.TestEnvDetails.NodeContainerEnvs == nil {
			state.Common.TestEnvDetails.NodeContainerEnvs = map[string]string{}
		}
		maps.Copy(state.Common.TestEnvDetails.NodeContainerEnvs, testenv)
	}

	state.DeployCluster(t, utils.ContractsDir)
	state.DeployContracts(utils.ContractsDir, subDir)

	gauntletCopyPath := utils.ProjectRoot + "/" + name
	if out, cpErr := exec.Command("cp", "-r", utils.ProjectRoot+"/gauntlet", gauntletCopyPath).Output(); cpErr != nil { // nolint:gosec
		require.NoError(t, err, "output: "+string(out))
	}

	sg, err := gauntlet.NewSolanaGauntlet(gauntletCopyPath)
	require.NoError(t, err)
	state.Gauntlet = sg

	state.SetupClients()
	require.NoError(t, err)

	gauntletConfig := map[string]string{
		"SECRET":      fmt.Sprintf("\"%s\"", *config.SolanaConfig.Secret),
		"NODE_URL":    state.Common.ChainDetails.RPCURLExternal,
		"WS_URL":      state.Common.ChainDetails.WSURLExternal,
		"PRIVATE_KEY": state.Common.AccountDetails.PrivateKey,
	}

	err = sg.SetupNetwork(gauntletConfig)
	require.NoError(t, err, "Error setting gauntlet network")
	err = sg.InstallDependencies()
	require.NoError(t, err, "Error installing gauntlet dependencies")

	if *config.Common.Network == client.DevnetGenesisHash {
		state.Common.ChainDetails.ProgramAddresses.OCR2 = *config.SolanaConfig.OCR2ProgramID
		state.Common.ChainDetails.ProgramAddresses.AccessController = *config.SolanaConfig.AccessControllerProgramID
		state.Common.ChainDetails.ProgramAddresses.Store = *config.SolanaConfig.StoreProgramID
		sg.LinkAddress = *config.SolanaConfig.LinkTokenAddress
		sg.VaultAddress = *config.SolanaConfig.VaultAddress
	} else {
		err = sg.DeployLinkToken()
		require.NoError(t, err)
	}

	err = sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "PROGRAM_ID_OCR2", state.Common.ChainDetails.ProgramAddresses.OCR2)
	require.NoError(t, err, "Error adding gauntlet variable")
	err = sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "PROGRAM_ID_ACCESS_CONTROLLER", state.Common.ChainDetails.ProgramAddresses.AccessController)
	require.NoError(t, err, "Error adding gauntlet variable")
	err = sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "PROGRAM_ID_STORE", state.Common.ChainDetails.ProgramAddresses.Store)
	require.NoError(t, err, "Error adding gauntlet variable")
	err = sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "LINK", sg.LinkAddress)
	require.NoError(t, err, "Error adding gauntlet variable")
	err = sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "VAULT_ADDRESS", sg.VaultAddress)
	require.NoError(t, err, "Error adding gauntlet variable")

	_, err = sg.DeployOCR2()
	require.NoError(t, err, "Error deploying OCR")
	ocr2Config := ocr_config.NewOCR2Config(state.Clients.ChainlinkClient.NKeys, sg.ProposalAddress, sg.VaultAddress, *config.SolanaConfig.Secret)
	ocr2Config.Default()
	sg.OCR2Config = ocr2Config

	err = sg.ConfigureOCR2()
	require.NoError(t, err)

	state.CreateJobs()

	envOut := &devenv.EnvOutput{
		OcrAddress:     sg.OcrAddress,
		FeedAddress:    sg.FeedAddress,
		RPCURLExternal: state.Common.ChainDetails.RPCURLExternal,
		WSURLExternal:  state.Common.ChainDetails.WSURLExternal,
		GauntletPath:   gauntletCopyPath,
	}
	_ = envOut.Write(envOutPath)

	return state, sg
}
