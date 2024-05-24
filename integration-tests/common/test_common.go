package common

import (
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"

	test_env_ctf "github.com/smartcontractkit/chainlink-testing-framework/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/testcontext"

	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/integration-tests/docker/test_env"

	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/store/models"

	test_env_sol "github.com/smartcontractkit/chainlink-solana/integration-tests/docker/testenv"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/testconfig"
)

type OCRv2TestState struct {
	ContractDeployer   *solclient.ContractDeployer
	LinkToken          *solclient.LinkToken
	ContractsNodeSetup map[int]*ContractNodeInfo
	Clients            *Clients
	Common             *Common
	Config             *Config
	Gauntlet           *gauntlet.SolanaGauntlet
}

type Clients struct {
	SolanaClient    *solclient.Client
	KillgraveClient *test_env_ctf.Killgrave
	ChainlinkClient *ChainlinkClient
}

type ChainlinkClient struct {
	ChainlinkClientDocker *test_env.ClCluster
	ChainlinkClientK8s    []*client.ChainlinkK8sClient
	ChainlinkNodes        []*client.ChainlinkClient
	NKeys                 []client.NodeKeysBundle
	AccountAddresses      []string
}

type Config struct {
	T          *testing.T
	TestConfig *testconfig.TestConfig
	Resty      *resty.Client
	err        error
}

func NewOCRv2State(t *testing.T, contracts int, namespacePrefix string, testConfig *testconfig.TestConfig) (*OCRv2TestState, error) {
	c, err := New(testConfig).Default(t, namespacePrefix)
	if err != nil {
		return nil, err
	}
	state := &OCRv2TestState{
		ContractsNodeSetup: make(map[int]*ContractNodeInfo),
		Common:             c,
		Clients: &Clients{
			SolanaClient:    &solclient.Client{},
			ChainlinkClient: &ChainlinkClient{},
		},
		Config: &Config{
			T:          t,
			TestConfig: testConfig,
			Resty:      nil,
			err:        nil,
		},
	}

	state.Clients.SolanaClient.Config = state.Clients.SolanaClient.Config.Default()
	for i := 0; i < contracts; i++ {
		state.ContractsNodeSetup[i] = &ContractNodeInfo{}
		state.ContractsNodeSetup[i].BootstrapNodeIdx = 0
		for n := 1; n < *state.Config.TestConfig.OCR2.NodeCount; n++ {
			state.ContractsNodeSetup[i].NodesIdx = append(state.ContractsNodeSetup[i].NodesIdx, n)
		}
	}
	return state, nil
}

type ContractsState struct {
	OCR           string `json:"ocr"`
	Store         string `json:"store"`
	Feed          string `json:"feed"`
	Owner         string `json:"owner"`
	Mint          string `json:"mint"`
	MintAuthority string `json:"mint_authority"`
	OCRVault      string `json:"ocr_vault"`
}

func (m *OCRv2TestState) DeployCluster(contractsDir string) {
	if *m.Config.TestConfig.Common.InsideK8s {
		m.DeployEnv(contractsDir)

		// Setting up the URLs
		m.Common.ChainDetails.RPCURLExternal = m.Common.Env.URLs["sol"][0]
		m.Common.ChainDetails.WSURLExternal = m.Common.Env.URLs["sol"][1]

		if *m.Config.TestConfig.Common.Network == "devnet" {
			m.Common.ChainDetails.RPCUrl = *m.Config.TestConfig.Common.RPCURL
			m.Common.ChainDetails.RPCURLExternal = *m.Config.TestConfig.Common.RPCURL
			m.Common.ChainDetails.WSURLExternal = *m.Config.TestConfig.Common.WsURL
		}

		m.Common.ChainDetails.MockserverURLInternal = m.Common.Env.URLs["qa_mock_adapter_internal"][0]
		m.Common.ChainDetails.MockServerEndpoint = "five"
	} else {
		env, err := test_env.NewTestEnv()
		require.NoError(m.Config.T, err)
		sol := test_env_sol.NewSolana([]string{env.DockerNetwork.Name}, *m.Config.TestConfig.Common.DevnetImage, m.Common.AccountDetails.PublicKey)
		err = sol.StartContainer()
		require.NoError(m.Config.T, err)

		// Setting the External RPC url for Gauntlet
		m.Common.ChainDetails.RPCUrl = sol.InternalHTTPURL
		m.Common.ChainDetails.RPCURLExternal = sol.ExternalHTTPURL
		m.Common.ChainDetails.WSURLExternal = sol.ExternalWsURL

		if *m.Config.TestConfig.Common.Network == "devnet" {
			m.Common.ChainDetails.RPCUrl = *m.Config.TestConfig.Common.RPCURL
			m.Common.ChainDetails.RPCURLExternal = *m.Config.TestConfig.Common.RPCURL
			m.Common.ChainDetails.WSURLExternal = *m.Config.TestConfig.Common.WsURL
		}

		b, err := test_env.NewCLTestEnvBuilder().
			WithNonEVM().
			WithTestInstance(m.Config.T).
			WithTestConfig(m.Config.TestConfig).
			WithMockAdapter().
			WithCLNodes(*m.Config.TestConfig.OCR2.NodeCount).
			WithCLNodeOptions(m.Common.TestEnvDetails.NodeOpts...).
			WithStandardCleanup().
			WithTestEnv(env)
		require.NoError(m.Config.T, err)
		env, err = b.Build()
		require.NoError(m.Config.T, err)
		m.Common.DockerEnv = &SolCLClusterTestEnv{
			CLClusterTestEnv: env,
			Sol:              sol,
			Killgrave:        env.MockAdapter,
		}
		// Setting up Mock adapter
		m.Clients.KillgraveClient = env.MockAdapter
		m.Common.ChainDetails.MockserverURLInternal = m.Clients.KillgraveClient.InternalEndpoint
		m.Common.ChainDetails.MockServerEndpoint = "mockserver-bridge"
		err = m.Clients.KillgraveClient.SetAdapterBasedIntValuePath("/mockserver-bridge", []string{http.MethodGet, http.MethodPost}, 5)
		require.NoError(m.Config.T, err, "Failed to set mock adapter value")
	}

	m.SetupClients()
	m.SetChainlinkNodes()
	m.DeployContracts(contractsDir)
}

// UploadProgramBinaries uploads programs binary files to solana-validator container
// currently it's the only way to deploy anything to local solana because ephemeral validator in k8s
// can't expose UDP ports required to copy .so chunks when deploying
func (m *OCRv2TestState) UploadProgramBinaries(contractsDir string) {
	pl, err := m.Common.Env.Client.ListPods(m.Common.Env.Cfg.Namespace, "app=sol")
	require.NoError(m.Config.T, err)
	_, _, _, err = m.Common.Env.Client.CopyToPod(m.Common.Env.Cfg.Namespace, contractsDir, fmt.Sprintf("%s/%s:/programs", m.Common.Env.Cfg.Namespace, pl.Items[0].Name), "sol-val")
	require.NoError(m.Config.T, err)
}

func (m *OCRv2TestState) DeployEnv(contractsDir string) {
	err := m.Common.Env.Run()
	require.NoError(m.Config.T, err)

	m.UploadProgramBinaries(contractsDir)
}

func (m *OCRv2TestState) NewSolanaClientSetup(networkSettings *solclient.SolNetwork) (*solclient.Client, error) {
	if *m.Config.TestConfig.Common.InsideK8s {
		networkSettings.URLs = m.Common.Env.URLs[networkSettings.Name]
	} else {
		networkSettings.URLs = []string{
			m.Common.DockerEnv.Sol.ExternalHTTPURL,
			m.Common.DockerEnv.Sol.ExternalWsURL,
		}
	}
	ec, err := solclient.NewClient(networkSettings)
	if err != nil {
		return nil, err
	}
	log.Info().
		Interface("URLs", networkSettings.URLs).
		Msg("Connected Solana client")
	return ec, nil
}

func (m *OCRv2TestState) SetupClients() {
	solClient, err := m.NewSolanaClientSetup(m.Clients.SolanaClient.Config)
	m.Clients.SolanaClient = solClient
	require.NoError(m.Config.T, err)
	if *m.Config.TestConfig.Common.InsideK8s {
		m.Clients.ChainlinkClient.ChainlinkClientK8s, err = client.ConnectChainlinkNodes(m.Common.Env)
		require.NoError(m.Config.T, err)
	} else {
		m.Clients.ChainlinkClient.ChainlinkClientDocker = m.Common.DockerEnv.ClCluster
	}
}

// DeployContracts deploys contracts
func (m *OCRv2TestState) DeployContracts(contractsDir string) {
	var err error
	m.Clients.ChainlinkClient.NKeys, err = m.Common.CreateNodeKeysBundle(m.Clients.ChainlinkClient.ChainlinkNodes)
	require.NoError(m.Config.T, err)
	cd, err := solclient.NewContractDeployer(m.Clients.SolanaClient, nil)
	require.NoError(m.Config.T, err)
	if *m.Config.TestConfig.Common.InsideK8s {
		err = cd.DeployAnchorProgramsRemote(contractsDir, m.Common.Env)
	} else {
		err = cd.DeployAnchorProgramsRemoteDocker(contractsDir, m.Common.DockerEnv.Sol)
	}
	require.NoError(m.Config.T, err)
}

// CreateJobs creating OCR jobs and EA stubs
func (m *OCRv2TestState) CreateJobs() {
	// Setting up RPC
	c := rpc.New(*m.Config.TestConfig.Common.RPCURL)
	wsc, err := ws.Connect(testcontext.Get(m.Config.T), *m.Config.TestConfig.Common.WsURL)
	require.NoError(m.Config.T, err, "Error connecting to websocket client")

	relayConfig := job.JSONConfig{
		"nodeEndpointHTTP": m.Common.ChainDetails.RPCUrl,
		"ocr2ProgramID":    m.Common.ChainDetails.ProgramAddresses.OCR2,
		"transmissionsID":  m.Gauntlet.FeedAddress,
		"storeProgramID":   m.Common.ChainDetails.ProgramAddresses.Store,
		"chainID":          m.Common.ChainDetails.ChainID,
	}
	boostratInternalIP := m.Clients.ChainlinkClient.ChainlinkNodes[0].InternalIP()
	bootstrapPeers := []client.P2PData{
		{
			InternalIP:   boostratInternalIP,
			InternalPort: "6690",
			PeerID:       m.Clients.ChainlinkClient.NKeys[0].PeerID,
		},
	}
	jobSpec := &client.OCR2TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.New().String()),
		JobType: "bootstrap",
		OCR2OracleSpec: job.OCR2OracleSpec{
			ContractID:                        m.Gauntlet.OcrAddress,
			Relay:                             m.Common.ChainDetails.ChainName,
			RelayConfig:                       relayConfig,
			P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
			OCRKeyBundleID:                    null.StringFrom(m.Clients.ChainlinkClient.NKeys[0].OCR2Key.Data.ID),
			TransmitterID:                     null.StringFrom(m.Clients.ChainlinkClient.NKeys[0].TXKey.Data.ID),
			ContractConfigConfirmations:       1,
			ContractConfigTrackerPollInterval: models.Interval(15 * time.Second),
		},
	}
	sourceValueBridge := client.BridgeTypeAttributes{
		Name:        "mockserver-bridge",
		URL:         fmt.Sprintf("%s/%s", m.Common.ChainDetails.MockserverURLInternal, m.Common.ChainDetails.MockServerEndpoint),
		RequestData: "{}",
	}

	observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
	bridgeInfo := BridgeInfo{ObservationSource: observationSource}

	err = m.Clients.ChainlinkClient.ChainlinkNodes[0].MustCreateBridge(&sourceValueBridge)
	require.NoError(m.Config.T, err, "Error creating bridge")

	_, err = m.Clients.ChainlinkClient.ChainlinkNodes[0].MustCreateJob(jobSpec)
	require.NoError(m.Config.T, err, "Error creating job")

	for nIdx, node := range m.Clients.ChainlinkClient.ChainlinkNodes {
		// Skipping bootstrap
		if nIdx == 0 {
			continue
		}
		if *m.Config.TestConfig.Common.Network == "localnet" {
			err = m.Clients.SolanaClient.Fund(m.Clients.ChainlinkClient.NKeys[nIdx].TXKey.Data.ID, big.NewFloat(1e4))
			require.NoError(m.Config.T, err, "Error sending funds")
		} else {
			err = solclient.SendFunds(*m.Config.TestConfig.Common.PrivateKey, m.Clients.ChainlinkClient.NKeys[nIdx].TXKey.Data.ID, 100000000, c, wsc)
			require.NoError(m.Config.T, err, "Error sending funds")
		}

		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        "mockserver-bridge",
			URL:         fmt.Sprintf("%s/%s", m.Common.ChainDetails.MockserverURLInternal, m.Common.ChainDetails.MockServerEndpoint),
			RequestData: "{}",
		}

		_, err := node.CreateBridge(&sourceValueBridge)
		require.NoError(m.Config.T, err, "Error creating bridge")

		jobSpec := &client.OCR2TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.New().String()),
			JobType:           "offchainreporting2",
			ObservationSource: bridgeInfo.ObservationSource,
			OCR2OracleSpec: job.OCR2OracleSpec{
				ContractID:                        m.Gauntlet.OcrAddress,
				Relay:                             m.Common.ChainDetails.ChainName,
				RelayConfig:                       relayConfig,
				P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
				OCRKeyBundleID:                    null.StringFrom(m.Clients.ChainlinkClient.NKeys[nIdx].OCR2Key.Data.ID),
				TransmitterID:                     null.StringFrom(m.Clients.ChainlinkClient.NKeys[nIdx].TXKey.Data.ID),
				ContractConfigConfirmations:       1,
				ContractConfigTrackerPollInterval: models.Interval(15 * time.Second),
				PluginType:                        "median",
				PluginConfig:                      PluginConfigToTomlFormat(observationSource),
			},
		}
		_, err = node.MustCreateJob(jobSpec)
		require.NoError(m.Config.T, err, "Error creating job")
	}
}

func (m *OCRv2TestState) SetChainlinkNodes() {
	// retrieve client from K8s client
	chainlinkNodes := []*client.ChainlinkClient{}
	if *m.Config.TestConfig.Common.InsideK8s {
		for i := range m.Clients.ChainlinkClient.ChainlinkClientK8s {
			chainlinkNodes = append(chainlinkNodes, m.Clients.ChainlinkClient.ChainlinkClientK8s[i].ChainlinkClient)
		}
	} else {
		chainlinkNodes = append(chainlinkNodes, m.Clients.ChainlinkClient.ChainlinkClientDocker.NodeAPIs()...)
	}
	m.Clients.ChainlinkClient.ChainlinkNodes = chainlinkNodes
}

func formatBuffer(buf []byte) string {
	if len(buf) == 0 {
		return ""
	}
	result := fmt.Sprintf("%d", buf[0])
	for _, b := range buf[1:] {
		result += fmt.Sprintf(",%d", b)
	}
	return result
}

func GetLatestRound(transmissions []gauntlet.Transmission) gauntlet.Transmission {
	highestRound := transmissions[0]
	for _, t := range transmissions[1:] {
		if t.RoundID > highestRound.RoundID {
			highestRound = t
		}
	}
	return highestRound
}
