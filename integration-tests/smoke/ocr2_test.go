package smoke

import (
	"fmt"
	"maps"
	"sort"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"gopkg.in/guregu/null.v4"

	tc "github.com/smartcontractkit/chainlink/integration-tests/testconfig"

	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/testcontext"
	"github.com/smartcontractkit/chainlink/integration-tests/actions"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/integration-tests/docker/test_env"
	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/store/models"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	for _, test := range []struct {
		name           string
		env            map[string]string
		enableGauntlet bool
	}{
		// {name: "embedded"},
		// {name: "plugins", env: map[string]string{
		// 	"CL_MEDIAN_CMD": "chainlink-feeds",
		// 	"CL_SOLANA_CMD": "chainlink-solana",
		// }},
		{name: "embedded-gauntlet", enableGauntlet: true},
	} {
		config, err := tc.GetConfig("Smoke", tc.OCR2)
		if err != nil {
			t.Fatal(err)
		}
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logging.Init()
			state, err := common.NewOCRv2State(t, 1, "smoke-"+test.name, "localnet", false, &config)
			require.NoError(t, err, "Could not setup the ocrv2 state")
			if len(test.env) > 0 {
				state.Common.NodeOpts = append(state.Common.NodeOpts, func(n *test_env.ClNode) {
					if n.ContainerEnvs == nil {
						n.ContainerEnvs = map[string]string{}
					}
					maps.Copy(n.ContainerEnvs, test.env)
				})
			}
			state.DeployCluster(utils.ContractsDir, test.enableGauntlet)

			state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
		})
	}
}

func TestSolanaGauntletOCRV2Smoke(t *testing.T) {
	config, err := tc.GetConfig("Smoke", tc.OCR2)
	if err != nil {
		t.Fatal(err)
	}
	l := logging.GetTestLogger(t)
	state, err := common.NewOCRv2State(t, 1, "gauntlet", "devnet", true, &config)
	require.NoError(t, err, "Could not setup the ocrv2 state")
	if state.Common.Env.WillUseRemoteRunner() {
		// run the remote runner and exit
		state.GauntletEnvToRemoteRunner()
		err := state.Common.Env.Run()
		require.NoError(t, err)
		return
	}
	sg, err := gauntlet.NewSolanaGauntlet(fmt.Sprintf("%s/gauntlet", utils.ProjectRoot))
	require.NoError(t, err)
	err = state.Common.Env.Run()
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := actions.TeardownSuite(t, state.Common.Env, state.ChainlinkNodesK8s, nil, zapcore.PanicLevel, nil); err != nil {
			l.Error().Err(err).Msg("Error tearing down environment")
		}
	})
	state.SetupClients(false) // not setting up gauntlet because it is set up outside
	state.NodeKeysBundle, err = state.Common.CreateNodeKeysBundle(state.GetChainlinkNodes())
	require.NoError(t, err)
	err = state.Common.CreateSolanaChainAndNode(state.GetChainlinkNodes())
	require.NoError(t, err)

	gauntletConfig := state.ConfigureGauntletFromEnv(utils.TestingSecret)
	err = sg.SetupNetwork(gauntletConfig)
	require.NoError(t, err, "Error setting gauntlet network")

	// Setting up RPC
	c := rpc.New(gauntletConfig["NODE_URL"])
	wsc, err := ws.Connect(testcontext.Get(t), gauntletConfig["WS_URL"])
	require.NoError(t, err)

	err = sg.DeployOCR2()
	require.NoError(t, err, "Error deploying OCR")

	bundleData := make([]client.NodeKeysBundle, len(state.NodeKeysBundle))
	copy(bundleData, state.NodeKeysBundle)

	// We have to sort by on_chain_pub_key for the config digest
	sort.Slice(bundleData, func(i, j int) bool {
		return bundleData[i].OCR2Key.Data.Attributes.OnChainPublicKey < bundleData[j].OCR2Key.Data.Attributes.OnChainPublicKey
	})

	onChainConfig, err := state.GenerateOnChainConfig(bundleData, gauntletConfig["VAULT"], sg.ProposalAddress)
	require.NoError(t, err)

	reportingConfig := utils.ReportingPluginConfig{
		AlphaReportInfinite: false,
		AlphaReportPpb:      0,
		AlphaAcceptInfinite: false,
		AlphaAcceptPpb:      0,
		DeltaCNanoseconds:   0,
	}
	offChainConfig := state.GenerateOffChainConfig(
		bundleData,
		sg.ProposalAddress,
		reportingConfig,
		int64(20000000000),
		int64(50000000000),
		int64(1000000000),
		int64(4000000000),
		int64(50000000000),
		3,
		int64(0),
		int64(3000000000),
		int64(3000000000),
		int64(100000000),
		int64(100000000),
		utils.TestingSecret,
	)

	payees := state.GeneratePayees(bundleData, gauntletConfig["VAULT"], sg.ProposalAddress)
	proposalAccept := state.GenerateProposalAcceptConfig(sg.ProposalAddress, 2, 1, onChainConfig.Oracles, offChainConfig.OffchainConfig, utils.TestingSecret)

	require.NoError(t, err)
	err = sg.ConfigureOCR2(onChainConfig, offChainConfig, payees, proposalAccept)
	require.NoError(t, err)

	err = state.Common.CreateSolanaChainAndNode(state.GetChainlinkNodes())
	require.NoError(t, err)

	// TODO - This needs to be decoupled into one method as in common.go
	// TODO - The current setup in common.go is using the solana validator, so we need to create one method for both gauntlet and solana
	// Leaving this for the time being as is so we have Testnet runs enabled on Solana
	relayConfig := job.JSONConfig{
		"nodeEndpointHTTP": state.Common.SolanaUrl,
		"ocr2ProgramID":    gauntletConfig["PROGRAM_ID_OCR2"],
		"transmissionsID":  sg.FeedAddress,
		"storeProgramID":   gauntletConfig["PROGRAM_ID_STORE"],
		"chainID":          state.Common.ChainId,
	}
	bootstrapPeers := []client.P2PData{
		{
			InternalIP:   state.ChainlinkNodesK8s[0].InternalIP(),
			InternalPort: "6690",
			PeerID:       state.NodeKeysBundle[0].PeerID,
		},
	}
	jobSpec := &client.OCR2TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.New().String()),
		JobType: "bootstrap",
		OCR2OracleSpec: job.OCR2OracleSpec{
			ContractID:                        sg.OcrAddress,
			Relay:                             common.ChainName,
			RelayConfig:                       relayConfig,
			P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
			OCRKeyBundleID:                    null.StringFrom(state.NodeKeysBundle[0].OCR2Key.Data.ID),
			TransmitterID:                     null.StringFrom(state.NodeKeysBundle[0].TXKey.Data.ID),
			ContractConfigConfirmations:       1,
			ContractConfigTrackerPollInterval: models.Interval(15 * time.Second),
		},
	}
	sourceValueBridge := client.BridgeTypeAttributes{
		Name:        "mockserver-bridge",
		URL:         fmt.Sprintf("%s/%s", state.Common.Env.URLs["qa_mock_adapter_internal"][0], "five"),
		RequestData: "{}",
	}

	observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
	bridgeInfo := common.BridgeInfo{ObservationSource: observationSource}
	err = state.ChainlinkNodesK8s[0].MustCreateBridge(&sourceValueBridge)
	require.NoError(t, err)
	_, err = state.ChainlinkNodesK8s[0].MustCreateJob(jobSpec)
	require.NoError(t, err)

	// TODO - This needs to be decoupled into one method as in common.go
	// TODO - The current setup in common.go is using the solana validator, so we need to create one method for both gauntlet and solana
	// Leaving this for the time being as is so we have Testnet runs enabled on Solana
	for nIdx, node := range state.ChainlinkNodesK8s {
		// Skipping bootstrap
		if nIdx == 0 {
			continue
		}
		err = solclient.SendFunds(gauntletConfig["PRIVATE_KEY"], state.NodeKeysBundle[nIdx].TXKey.Data.ID, 100000000, c, wsc)
		require.NoError(t, err, "Error sending Funds")
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        "mockserver-bridge",
			URL:         fmt.Sprintf("%s/%s", state.Common.Env.URLs["qa_mock_adapter_internal"][0], "five"),
			RequestData: "{}",
		}
		_, err := node.CreateBridge(&sourceValueBridge)
		require.NoError(t, err)
		jobSpec := &client.OCR2TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.New().String()),
			JobType:           "offchainreporting2",
			ObservationSource: bridgeInfo.ObservationSource,
			OCR2OracleSpec: job.OCR2OracleSpec{
				ContractID:                        sg.OcrAddress,
				Relay:                             common.ChainName,
				RelayConfig:                       relayConfig,
				P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
				OCRKeyBundleID:                    null.StringFrom(state.NodeKeysBundle[nIdx].OCR2Key.Data.ID),
				TransmitterID:                     null.StringFrom(state.NodeKeysBundle[nIdx].TXKey.Data.ID),
				ContractConfigConfirmations:       1,
				ContractConfigTrackerPollInterval: models.Interval(15 * time.Second),
				PluginType:                        "median",
				PluginConfig:                      common.PluginConfigToTomlFormat(observationSource),
			},
		}
		_, err = node.MustCreateJob(jobSpec)
		require.NoError(t, err)

	}

	// Test start
	for i := 1; i < 10; i++ {
		transmissions, err := sg.FetchTransmissions(sg.OcrAddress)
		require.NoError(t, err)
		if len(transmissions) <= 1 {
			l.Info().Str("Contract", sg.OcrAddress).Str("No", "Transmissions")
		} else {
			l.Info().Str("Contract", sg.OcrAddress).Interface("Answer", transmissions[0].Answer).Int64("RoundID", transmissions[0].RoundId).Msg("New answer found")
			assert.Equal(t, transmissions[0].Answer, int64(5), fmt.Sprintf("Actual: %d, Expected: 5", transmissions[0].Answer))
			assert.Less(t, transmissions[1].RoundId, transmissions[0].RoundId, fmt.Sprintf("Expected round %d to be less than %d", transmissions[1].RoundId, transmissions[0].RoundId))
		}
		time.Sleep(time.Second * 6)
	}
}
