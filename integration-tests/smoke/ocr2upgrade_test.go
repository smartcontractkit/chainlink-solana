package smoke

import (
	"fmt"
	"maps"
	"os/exec"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	ocr_config "github.com/smartcontractkit/chainlink-solana/integration-tests/config"
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

	state, sg := setupOCR2UpgradeEnvironment(t, name, env, config, "previous")

	validateUpgradeRounds(t, name, sg.OcrAddress, sg, *config.OCR2.NumberOfRounds)

	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|           REDEPLOYING CONTRACTS           |")
	log.Info().Msg("---------------------------------------------")
	state.UpgradeContracts(utils.ContractsDir, "")
	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|                                           |")
	log.Info().Msg("---------------------------------------------")

	validateUpgradeRounds(t, name, sg.OcrAddress, sg, *config.OCR2.NumberOfRounds)
}

func setupOCR2UpgradeEnvironment(t *testing.T, testname string, testenv map[string]string, config tc.TestConfig, subDir string) (*common.OCRv2TestState, *gauntlet.SolanaGauntlet) {
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
	if out, cpErr := exec.Command("cp", "-r", utils.ProjectRoot+"/gauntlet", gauntletCopyPath).Output(); cpErr != nil { //nolint:gosec
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

	return state, sg
}

func validateUpgradeRounds(t *testing.T, testname string, ocrAddress string, sg *gauntlet.SolanaGauntlet, rounds int) {
	t.Helper()
	stuck := 0
	successfulRounds := 0
	prevRound := gauntlet.Transmission{RoundID: 0}

	for successfulRounds < rounds {
		time.Sleep(6 * time.Second)
		require.Less(t, stuck, 10, fmt.Sprintf("%s: Rounds have been stuck for more than 10 iterations", testname))

		log.Info().Str("Transmission", ocrAddress).Msg("Inspecting transmissions")
		transmissions, err := sg.FetchTransmissions(ocrAddress)
		require.NoError(t, err)
		if len(transmissions) <= 1 {
			log.Info().Str("Contract", ocrAddress).Msg(fmt.Sprintf("%s: No Transmissions", testname))
			stuck++
			continue
		}
		currentRound := common.GetLatestRound(transmissions)
		if prevRound.RoundID == 0 {
			prevRound = currentRound
		}
		if currentRound.RoundID <= prevRound.RoundID {
			log.Info().Str("Transmission", ocrAddress).Msg(fmt.Sprintf("%s: No new transmissions", testname))
			stuck++
			continue
		}
		log.Info().Str("Contract", ocrAddress).Interface("Answer", currentRound.Answer).Int64("RoundID", currentRound.RoundID).Msg(fmt.Sprintf("%s: New answer found", testname))
		require.Equal(t, int64(5), currentRound.Answer, fmt.Sprintf("Actual: %d, Expected: 5", currentRound.Answer))
		require.Less(t, prevRound.RoundID, currentRound.RoundID)
		prevRound = currentRound
		successfulRounds++
		stuck = 0
	}
}
