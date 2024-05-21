package smoke

import (
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	ocr_config "github.com/smartcontractkit/chainlink-solana/integration-tests/config"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	tc "github.com/smartcontractkit/chainlink-solana/integration-tests/testconfig"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink/integration-tests/actions"
	"github.com/smartcontractkit/chainlink/integration-tests/docker/test_env"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
	}{
		{name: "embedded"},
		{name: "plugins", env: map[string]string{
			"CL_MEDIAN_CMD": "chainlink-feeds",
			"CL_SOLANA_CMD": "chainlink-solana",
		}},
	} {
		config, err := tc.GetConfig("Smoke", tc.OCR2)
		if err != nil {
			t.Fatal(err)
		}

		test := test
		t.Run(test.name, func(t *testing.T) {

			state, err := common.NewOCRv2State(t, 1, "gauntlet-"+test.name, &config)
			require.NoError(t, err, "Could not setup the ocrv2 state")
			if len(test.env) > 0 {
				state.Common.TestEnvDetails.NodeOpts = append(state.Common.TestEnvDetails.NodeOpts, func(n *test_env.ClNode) {
					if n.ContainerEnvs == nil {
						n.ContainerEnvs = map[string]string{}
					}
					maps.Copy(n.ContainerEnvs, test.env)
				})
			}

			state.DeployCluster(utils.ContractsDir)

			sg, err := gauntlet.NewSolanaGauntlet(fmt.Sprintf("%s/gauntlet", utils.ProjectRoot))
			require.NoError(t, err)
			state.Gauntlet = sg

			if *config.Common.InsideK8s {
				t.Cleanup(func() {
					if err := actions.TeardownRemoteSuite(t, state.Common.Env.Cfg.Namespace, state.Clients.ChainlinkClient.ChainlinkClientK8s, nil, nil, nil); err != nil {
						log.Error().Err(err).Msg("Error tearing down environment")
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

			if *config.Common.Network == "devnet" {
				state.Common.ChainDetails.ProgramAddresses.OCR2 = *config.SolanaConfig.OCR2ProgramId
				state.Common.ChainDetails.ProgramAddresses.AccessController = *config.SolanaConfig.AccessControllerProgramId
				state.Common.ChainDetails.ProgramAddresses.Store = *config.SolanaConfig.StoreProgramId
				sg.LinkAddress = *config.SolanaConfig.LinkTokenAddress
				sg.VaultAddress = *config.SolanaConfig.VaultAddress
			} else {
				// Deploying LINK in case of localnet
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
			// Generating default OCR2 config
			ocr2Config := ocr_config.NewOCR2Config(state.Clients.ChainlinkClient.NKeys, sg.ProposalAddress, sg.VaultAddress, *config.SolanaConfig.Secret)
			ocr2Config.Default()
			sg.OCR2Config = ocr2Config

			err = sg.ConfigureOCR2()
			require.NoError(t, err)

			state.CreateJobs()

			// Test start
			stuck := 0
			successFullRounds := 0
			prevRound := gauntlet.Transmission{
				RoundId: 0,
			}
			for successFullRounds < *config.OCR2.Smoke.NumberOfRounds {
				require.Less(t, stuck, 10, "Rounds have been stuck for more than 10 iterations")
				log.Info().Str("Transmission", sg.OcrAddress).Msg("Inspecting transmissions")
				transmissions, err := sg.FetchTransmissions(sg.OcrAddress)
				require.NoError(t, err)
				if len(transmissions) <= 1 {
					log.Info().Str("Contract", sg.OcrAddress).Str("No", "Transmissions")
					stuck++
					continue
				}
				currentRound := common.GetLatestRound(transmissions)
				if prevRound.RoundId == 0 {
					prevRound = currentRound
				}
				if currentRound.RoundId <= prevRound.RoundId {
					log.Info().Str("Transmission", sg.OcrAddress).Msg("No new transmissions")
					stuck++
					continue
				}
				log.Info().Str("Contract", sg.OcrAddress).Interface("Answer", currentRound.Answer).Int64("RoundID", currentRound.Answer).Msg("New answer found")
				require.Equal(t, currentRound.Answer, int64(5), fmt.Sprintf("Actual: %d, Expected: 5", currentRound.Answer))
				require.Less(t, prevRound.RoundId, currentRound.RoundId, fmt.Sprintf("Expected round %d to be less than %d", prevRound.RoundId, currentRound.RoundId))
				prevRound = currentRound
				successFullRounds++
				time.Sleep(time.Second * 6)
				stuck = 0
			}
		})

	}
}
