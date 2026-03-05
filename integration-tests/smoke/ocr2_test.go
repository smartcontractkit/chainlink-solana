// OCR2 smoke tests -- migrated from chainlink/integration-tests test_env + nodeclient
// to CTF simple_node_set + clclient (Phase 1 of Solana test decoupling).
// Previous dependencies on chainlink/integration-tests, chainlink/deployment,
// and chainlink/v2 have been removed.
package smoke

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

func TestSolanaOCRV2Smoke(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "embedded"},
		{name: "plugins", env: map[string]string{
			"CL_MEDIAN_CMD": "chainlink-feeds",
			"CL_SOLANA_CMD": "chainlink-solana",
		}},
	}

	for idx := range tests {
		test := tests[idx]

		config, err := tc.GetConfig("Smoke", tc.OCR2)
		if err != nil {
			t.Fatal(err)
		}

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			envOutPath := filepath.Join(os.TempDir(), fmt.Sprintf("sol-ocr2-%s-%s", test.name, devenv.DefaultEnvOutFile))

			envOut, sg := setupOCR2Environment(t, test.name, test.env, config, "", envOutPath)
			validateRoundsFromEnv(t, test.name, envOut, sg, *config.OCR2.NumberOfRounds)
		})
	}
}

// setupOCR2Environment is the setup phase: deploys cluster, contracts, creates
// jobs, and writes the environment output to envOutPath. Returns the env output
// and gauntlet instance for the assertion phase.
func setupOCR2Environment(t *testing.T, testname string, testenv map[string]string, config tc.TestConfig, subDir, envOutPath string) (*devenv.EnvOutput, *gauntlet.SolanaGauntlet) {
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
	if state.Common.Env != nil && state.Common.Env.WillUseRemoteRunner() {
		return nil, nil
	}

	gauntletCopyPath := utils.ProjectRoot + "/" + name
	if out, cpErr := exec.Command("cp", "-r", utils.ProjectRoot+"/gauntlet", gauntletCopyPath).Output(); cpErr != nil { // nolint:gosec
		require.NoError(t, err, "output: "+string(out))
	}

	sg, err := gauntlet.NewSolanaGauntlet(gauntletCopyPath)
	require.NoError(t, err)
	state.Gauntlet = sg

	if *config.Common.InsideK8s {
		t.Cleanup(func() {
			err = state.Common.Env.Shutdown()
			if err != nil {
				log.Err(err)
			}
		})
	}

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
	err = envOut.Write(envOutPath)
	require.NoError(t, err, "Failed to write env output")
	log.Info().Str("path", envOutPath).Msg("Wrote env-out.toml")

	return envOut, sg
}

// validateRoundsFromEnv is the assertion phase: uses the environment output and
// gauntlet to validate that OCR rounds are progressing.
func validateRoundsFromEnv(t *testing.T, testname string, envOut *devenv.EnvOutput, sg *gauntlet.SolanaGauntlet, rounds int) {
	if envOut == nil || sg == nil {
		return
	}
	name := "gauntlet" + testname
	ocrAddress := envOut.OcrAddress

	stuck := 0
	successFullRounds := 0
	prevRound := gauntlet.Transmission{
		RoundID: 0,
	}
	for successFullRounds < rounds {
		time.Sleep(time.Second * 6)
		require.Less(t, stuck, 10, fmt.Sprintf("%s: Rounds have been stuck for more than 10 iterations", name))
		log.Info().Str("Transmission", ocrAddress).Msg("Inspecting transmissions")
		transmissions, err := sg.FetchTransmissions(ocrAddress)
		require.NoError(t, err)
		if len(transmissions) <= 1 {
			log.Info().Str("Contract", ocrAddress).Msg(fmt.Sprintf("%s: No Transmissions", name))
			stuck++
			continue
		}
		currentRound := common.GetLatestRound(transmissions)
		if prevRound.RoundID == 0 {
			prevRound = currentRound
		}
		if currentRound.RoundID <= prevRound.RoundID {
			log.Info().Str("Transmission", ocrAddress).Msg(fmt.Sprintf("%s: No new transmissions", name))
			stuck++
			continue
		}
		log.Info().Str("Contract", ocrAddress).Interface("Answer", currentRound.Answer).Int64("RoundID", currentRound.RoundID).Msg(fmt.Sprintf("%s: New answer found", name))
		require.Equal(t, currentRound.Answer, int64(5), fmt.Sprintf("Actual: %d, Expected: 5", currentRound.Answer))
		require.Less(t, prevRound.RoundID, currentRound.RoundID, fmt.Sprintf("Expected round %d to be less than %d", prevRound.RoundID, currentRound.RoundID))
		prevRound = currentRound
		successFullRounds++
		stuck = 0
	}
}
