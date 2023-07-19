package smoke

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	ctfUtils "github.com/smartcontractkit/chainlink-testing-framework/utils"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/store/models"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"
	"sort"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	"github.com/stretchr/testify/require"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	state := common.NewOCRv2State(t, 1, "smoke", "localnet")
	state.DeployCluster(utils.ContractsDir)
	if state.Common.Env.WillUseRemoteRunner() {
		return
	}
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
}

func TestSolanaGauntletOCRV2Smoke(t *testing.T) {
	secret := "this is an testing only secret"
	state := common.NewOCRv2State(t, 1, "gauntlet", "devnet")
	sg, err := gauntlet.NewSolanaGauntlet(fmt.Sprintf("%s/gauntlet", utils.ProjectRoot))

	err = state.Common.Env.Run()
	require.NoError(t, err)

	state.SetupClients()
	state.NodeKeysBundle, err = state.Common.CreateNodeKeysBundle(state.ChainlinkNodes)
	require.NoError(t, err)
	err = state.Common.CreateSolanaChainAndNode(state.ChainlinkNodes)
	require.NoError(t, err)

	gauntletConfig := state.ConfigureGauntlet(secret)
	err = sg.SetupNetwork(gauntletConfig)
	require.NoError(t, err, "Error setting gauntlet network")

	// Setting up RPC
	c := rpc.New(gauntletConfig["NODE_URL"])
	wsc, err := ws.Connect(context.Background(), gauntletConfig["WS_URL"])

	_, err = sg.DeployOCR2()
	require.NoError(t, err, "Error deploying OCR")

	bundleData := make([]client.NodeKeysBundle, len(state.NodeKeysBundle))
	copy(bundleData, state.NodeKeysBundle)

	// We have to sort by on_chain_pub_key for the config digest
	sort.Slice(bundleData, func(i, j int) bool {
		return bundleData[i].OCR2Key.Data.Attributes.OnChainPublicKey < bundleData[j].OCR2Key.Data.Attributes.OnChainPublicKey
	})

	onChainConfig, err := state.GenerateOnChainConfig(bundleData, gauntletConfig["VAULT"], sg.ProposalAddress)
	require.NoError(t, err)

	reportingConfig := common.ReportingPluginConfig{
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
		secret,
	)

	payees := state.GeneratePayees(bundleData, gauntletConfig["VAULT"], sg.ProposalAddress)
	proposalAccept := state.GenerateProposalAcceptConfig(sg.ProposalAddress, 2, 1, onChainConfig.Oracles, offChainConfig.OffchainConfig, secret)

	require.NoError(t, err)
	err = sg.ConfigureOCR2(onChainConfig, offChainConfig, payees, proposalAccept)

	err = state.Common.CreateSolanaChainAndNode(state.ChainlinkNodes)
	require.NoError(t, err)
	err = state.MockServer.SetValuePath("/juels", 1)
	require.NoError(t, err)

	// TODO - This needs to be decoupled into one method as in common.go
	// TODO - The current setup in common.go is using the solana validator, so we need to create one method for both gauntlet and solana
	// Leaving this for the time being as is so we have Testnet runs enabled on Solana
	relayConfig := job.JSONConfig{
		"nodeEndpointHTTP": fmt.Sprintf("\"%s\"", state.Common.SolanaUrl),
		"ocr2ProgramID":    fmt.Sprintf("\"%s\"", gauntletConfig["PROGRAM_ID_OCR2"]),
		"transmissionsID":  fmt.Sprintf("\"%s\"", sg.FeedAddress),
		"storeProgramID":   fmt.Sprintf("\"%s\"", gauntletConfig["PROGRAM_ID_STORE"]),
		"chainID":          fmt.Sprintf("\"%s\"", state.Common.ChainId),
	}
	bootstrapPeers := []client.P2PData{
		{
			InternalIP:   state.ChainlinkNodes[0].InternalIP(),
			InternalPort: "6690",
			PeerID:       state.NodeKeysBundle[0].PeerID,
		},
	}
	jobSpec := &client.OCR2TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.NewV4().String()),
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
		URL:         fmt.Sprintf("%s/%s", state.MockServer.Config.ClusterURL, "juels"),
		RequestData: "{}",
	}

	observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
	bridgeInfo := common.BridgeInfo{ObservationSource: observationSource}
	err = state.ChainlinkNodes[0].MustCreateBridge(&sourceValueBridge)
	require.NoError(t, err)
	_, err = state.ChainlinkNodes[0].MustCreateJob(jobSpec)
	require.NoError(t, err)

	// TODO - This needs to be decoupled into one method as in common.go
	// TODO - The current setup in common.go is using the solana validator, so we need to create one method for both gauntlet and solana
	// Leaving this for the time being as is so we have Testnet runs enabled on Solana
	for nIdx, node := range state.ChainlinkNodes {
		// Skipping bootstrap
		if nIdx == 0 {
			continue
		}
		err = solclient.SendFunds(gauntletConfig["PRIVATE_KEY"], state.NodeKeysBundle[nIdx].TXKey.Data.ID, 100000000, c, wsc)
		require.NoError(t, err, "Error sending Funds")
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        "mockserver-bridge",
			URL:         fmt.Sprintf("%s/%s", state.MockServer.Config.ClusterURL, "juels"),
			RequestData: "{}",
		}
		_, err := node.CreateBridge(&sourceValueBridge)
		require.NoError(t, err)
		jobSpec := &client.OCR2TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
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
	l := ctfUtils.GetTestLogger(t)
	for i := 1; i < 10; i++ {
		transmissions, err := sg.FetchTransmissions(sg.OcrAddress)
		require.NoError(t, err)
		if len(transmissions) <= 1 {
			l.Info().Str("Contract", sg.OcrAddress).Str("No", "Transmissions")
		} else {
			l.Info().Str("Contract", sg.OcrAddress).Interface("Answer", transmissions[0].Answer).Int64("RoundID", transmissions[0].RoundId).Msg("New answer found")
			assert.Equal(t, transmissions[0].Answer, int64(1), fmt.Sprintf("Actual: %d, Expected: 1", transmissions[0].Answer))
			assert.Less(t, transmissions[1].RoundId, transmissions[0].RoundId, fmt.Sprintf("Expected round %d to be less than %d", transmissions[1].RoundId, transmissions[0].RoundId))
		}
		time.Sleep(time.Second * 6)
	}
}
