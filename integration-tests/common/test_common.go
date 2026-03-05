package common

import (
	"fmt"
	"math/big"
	"net/http"
	"path/filepath"
	"strings"
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

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/clclient"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/clnode"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/postgres"
	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"
	testenvctf "github.com/smartcontractkit/chainlink-testing-framework/lib/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/utils/testcontext"
	"github.com/smartcontractkit/chainlink-testing-framework/parrot"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv"
	testenvsol "github.com/smartcontractkit/chainlink-solana/integration-tests/docker/testenv"
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
	ParrotClient    *testenvctf.Parrot
	ChainlinkClient *ChainlinkClient
}

type ChainlinkClient struct {
	ChainlinkNodes   []*clclient.ChainlinkClient
	NKeys            []clclient.NodeKeysBundle
	AccountAddresses []string
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

func (m *OCRv2TestState) DeployCluster(t *testing.T, contractsDir string) {
	if *m.Config.TestConfig.Common.InsideK8s {
		m.DeployEnv(contractsDir)

		if m.Common.Env.WillUseRemoteRunner() {
			return
		}

		m.Common.ChainDetails.RPCURLExternal = m.Common.Env.URLs["sol"][0]
		m.Common.ChainDetails.WSURLExternal = m.Common.Env.URLs["sol"][1]

		if *m.Config.TestConfig.Common.Network == "devnet" {
			m.Common.ChainDetails.RPCUrls = *m.Config.TestConfig.Common.RPCURLs
			m.Common.ChainDetails.RPCURLExternal = (*m.Config.TestConfig.Common.RPCURLs)[0]
			m.Common.ChainDetails.WSURLExternal = (*m.Config.TestConfig.Common.WsURLs)[0]
		}

		m.Common.ChainDetails.MockserverURLInternal = m.Common.Env.URLs["qa_mock_adapter_internal"][0]
		m.Common.ChainDetails.MockServerEndpoint = "five"
	} else {
		err := framework.DefaultNetwork(nil)
		require.NoError(m.Config.T, err)

		sol := testenvsol.NewSolana(
			[]string{framework.DefaultNetworkName},
			*m.Config.TestConfig.Common.DevnetImage,
			m.Common.AccountDetails.PublicKey,
		).WithTestLogger(t)
		err = sol.StartContainer()
		require.NoError(m.Config.T, err)

		m.Common.ChainDetails.RPCUrls = []string{sol.InternalHTTPURL}
		m.Common.ChainDetails.RPCURLExternal = sol.ExternalHTTPURL
		m.Common.ChainDetails.WSURLExternal = sol.ExternalWsURL

		if *m.Config.TestConfig.Common.Network == "devnet" {
			m.Common.ChainDetails.RPCUrls = *m.Config.TestConfig.Common.RPCURLs
			m.Common.ChainDetails.RPCURLExternal = (*m.Config.TestConfig.Common.RPCURLs)[0]
			m.Common.ChainDetails.WSURLExternal = (*m.Config.TestConfig.Common.WsURLs)[0]
		}

		mockAdapter := testenvctf.NewParrot([]string{framework.DefaultNetworkName}).WithTestInstance(t)
		err = mockAdapter.StartContainer()
		require.NoError(m.Config.T, err)

		nodeConfigTOML, err := m.Config.TestConfig.GetNodeConfigTOML()
		require.NoError(m.Config.T, err)

		clImage := fmt.Sprintf("%s:%s", *m.Config.TestConfig.ChainlinkImage.Image, *m.Config.TestConfig.ChainlinkImage.Version)
		nodeCount := *m.Config.TestConfig.OCR2.NodeCount
		nodeSpecs := make([]*clnode.Input, nodeCount)
		for i := 0; i < nodeCount; i++ {
			nodeSpecs[i] = &clnode.Input{
				Node: &clnode.NodeInput{
					Image:               clImage,
					TestConfigOverrides: nodeConfigTOML,
					EnvVars:             m.Common.TestEnvDetails.NodeContainerEnvs,
				},
			}
		}

		testNameParts := strings.Split(t.Name(), "/")
		nodeSetSuffix := strings.ToLower(testNameParts[len(testNameParts)-1])

		nodeSetInput := &ns.Input{
			Name:               fmt.Sprintf("sol-ocr-%s", nodeSetSuffix),
			Nodes:              nodeCount,
			OverrideMode:       "each",
			HTTPPortRangeStart: 20000,
			P2PPortRangeStart:  22000,
			DbInput: &postgres.Input{
				Image: "postgres:15",
			},
			NodeSpecs: nodeSpecs,
		}

		nsOut, err := ns.NewSharedDBNodeSet(nodeSetInput, nil)
		require.NoError(m.Config.T, err)

		clients := make([]*clclient.ChainlinkClient, len(nsOut.CLNodes))
		for i, n := range nsOut.CLNodes {
			c, err := clclient.NewChainlinkClient(&clclient.Config{
				URL:        n.Node.ExternalURL,
				Email:      n.Node.APIAuthUser,
				Password:   n.Node.APIAuthPassword,
				InternalIP: n.Node.InternalIP,
			})
			require.NoError(m.Config.T, err)
			clients[i] = c
		}

		m.Common.DockerEnv = &SolCLClusterTestEnv{
			Sol:     sol,
			Parrot:  mockAdapter,
			Clients: clients,
			NodeSet: nsOut,
		}

		m.Clients.ParrotClient = mockAdapter
		m.Common.ChainDetails.MockserverURLInternal = mockAdapter.InternalEndpoint
		m.Common.ChainDetails.MockServerEndpoint = "mockserver-bridge"
		err = m.Clients.ParrotClient.SetAdapterRoute(&parrot.Route{
			Path:               "/mockserver-bridge",
			Method:             http.MethodGet,
			ResponseBody:       5,
			ResponseStatusCode: http.StatusOK,
		})
		require.NoError(m.Config.T, err, "Failed to set mock adapter value")
		err = m.Clients.ParrotClient.SetAdapterRoute(&parrot.Route{
			Path:               "/mockserver-bridge",
			Method:             http.MethodPost,
			ResponseBody:       5,
			ResponseStatusCode: http.StatusOK,
		})
		require.NoError(m.Config.T, err, "Failed to set mock adapter value")
	}

	m.SetupClients()
	m.SetChainlinkNodes()
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

	if !m.Common.Env.WillUseRemoteRunner() {
		m.UploadProgramBinaries(contractsDir)
	}
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
}

// DeployContracts deploys contracts
// baseDir is the root folder where contracts are stored
// subDir allows for pointing to a subdirectory within baseDir (can be left empty)
func (m *OCRv2TestState) DeployContracts(baseDir, subDir string) {
	var err error
	m.Clients.ChainlinkClient.NKeys, err = m.Common.CreateNodeKeysBundle(m.Clients.ChainlinkClient.ChainlinkNodes)
	require.NoError(m.Config.T, err)
	cd, err := solclient.NewContractDeployer(m.Clients.SolanaClient, nil)
	require.NoError(m.Config.T, err)
	if *m.Config.TestConfig.Common.InsideK8s {
		err = cd.DeployAnchorProgramsRemote(baseDir, m.Common.Env)
	} else {
		err = cd.DeployAnchorProgramsRemoteDocker(baseDir, subDir, m.Common.DockerEnv.Sol, solclient.BuildProgramIDKeypairPath)
	}
	require.NoError(m.Config.T, err)
}

func (m *OCRv2TestState) UpgradeContracts(baseDir, subDir string) {
	cd, err := solclient.NewContractDeployer(m.Clients.SolanaClient, nil)
	require.NoError(m.Config.T, err)

	programIDBuilder := func(programName string) string {
		programName, _ = strings.CutSuffix(filepath.Base(programName), ".so")
		ids := map[string]string{
			"ocr_2":             m.Common.ChainDetails.ProgramAddresses.OCR2,
			"access_controller": m.Common.ChainDetails.ProgramAddresses.AccessController,
			"store":             m.Common.ChainDetails.ProgramAddresses.Store,
		}
		val, ok := ids[programName]
		if !ok {
			val = solclient.BuildProgramIDKeypairPath(programName)
			log.Warn().Str("Program", programName).Msg(fmt.Sprintf("falling back to path (%s) unable to find corresponding key (%s) within %+v", val, programName, ids))
		}
		return val
	}

	if *m.Config.TestConfig.Common.InsideK8s {
		err = fmt.Errorf("not implemented")
	} else {
		err = cd.DeployAnchorProgramsRemoteDocker(baseDir, subDir, m.Common.DockerEnv.Sol, programIDBuilder)
	}
	require.NoError(m.Config.T, err)
}

// CreateJobs creating OCR jobs and EA stubs
func (m *OCRv2TestState) CreateJobs() {
	c := rpc.New(m.Common.ChainDetails.RPCURLExternal)
	wsc, err := ws.Connect(testcontext.Get(m.Config.T), m.Common.ChainDetails.WSURLExternal)
	require.NoError(m.Config.T, err, "Error connecting to websocket client")

	relayConfig := devenv.JSONConfig{
		"nodeEndpointHTTP": m.Common.ChainDetails.RPCUrls,
		"ocr2ProgramID":    m.Common.ChainDetails.ProgramAddresses.OCR2,
		"transmissionsID":  m.Gauntlet.FeedAddress,
		"storeProgramID":   m.Common.ChainDetails.ProgramAddresses.Store,
		"chainID":          m.Common.ChainDetails.ChainID,
	}
	boostratInternalIP := m.Clients.ChainlinkClient.ChainlinkNodes[0].InternalIP()
	bootstrapPeers := []clclient.P2PData{
		{
			InternalIP:   boostratInternalIP,
			InternalPort: "6690",
			PeerID:       m.Clients.ChainlinkClient.NKeys[0].PeerID,
		},
	}
	jobSpec := &devenv.TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.New().String()),
		JobType: "bootstrap",
		OCR2OracleSpec: devenv.OracleSpec{
			ContractID:                        m.Gauntlet.OcrAddress,
			Relay:                             m.Common.ChainDetails.ChainName,
			RelayConfig:                       relayConfig,
			P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
			OCRKeyBundleID:                    null.StringFrom(m.Clients.ChainlinkClient.NKeys[0].OCR2Key.Data.ID),
			TransmitterID:                     null.StringFrom(m.Clients.ChainlinkClient.NKeys[0].TXKey.Data.ID),
			ContractConfigConfirmations:       1,
			ContractConfigTrackerPollInterval: *devenv.NewInterval(15 * time.Second),
		},
	}
	sourceValueBridge := clclient.BridgeTypeAttributes{
		Name:        "mockserver-bridge",
		URL:         fmt.Sprintf("%s/%s", m.Common.ChainDetails.MockserverURLInternal, m.Common.ChainDetails.MockServerEndpoint),
		RequestData: "{}",
	}

	observationSource := clclient.ObservationSourceSpecBridge(&sourceValueBridge)
	bridgeInfo := BridgeInfo{ObservationSource: observationSource}

	err = m.Clients.ChainlinkClient.ChainlinkNodes[0].MustCreateBridge(&sourceValueBridge)
	require.NoError(m.Config.T, err, "Error creating bridge")

	_, err = m.Clients.ChainlinkClient.ChainlinkNodes[0].MustCreateJob(jobSpec)
	require.NoError(m.Config.T, err, "Error creating job")

	for nIdx, node := range m.Clients.ChainlinkClient.ChainlinkNodes {
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

		sourceValueBridge := clclient.BridgeTypeAttributes{
			Name:        "mockserver-bridge",
			URL:         fmt.Sprintf("%s/%s", m.Common.ChainDetails.MockserverURLInternal, m.Common.ChainDetails.MockServerEndpoint),
			RequestData: "{}",
		}

		_, err := node.CreateBridge(&sourceValueBridge)
		require.NoError(m.Config.T, err, "Error creating bridge")

		jobSpec := &devenv.TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.New().String()),
			JobType:           "offchainreporting2",
			ObservationSource: bridgeInfo.ObservationSource,
			OCR2OracleSpec: devenv.OracleSpec{
				ContractID:                        m.Gauntlet.OcrAddress,
				Relay:                             m.Common.ChainDetails.ChainName,
				RelayConfig:                       relayConfig,
				P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
				OCRKeyBundleID:                    null.StringFrom(m.Clients.ChainlinkClient.NKeys[nIdx].OCR2Key.Data.ID),
				TransmitterID:                     null.StringFrom(m.Clients.ChainlinkClient.NKeys[nIdx].TXKey.Data.ID),
				ContractConfigConfirmations:       1,
				ContractConfigTrackerPollInterval: *devenv.NewInterval(15 * time.Second),
				PluginType:                        "median",
				PluginConfig:                      PluginConfigToTomlFormat(observationSource),
			},
		}
		_, err = node.MustCreateJob(jobSpec)
		require.NoError(m.Config.T, err, "Error creating job")
	}
}

func (m *OCRv2TestState) SetChainlinkNodes() {
	if m.Common.DockerEnv != nil && len(m.Common.DockerEnv.Clients) > 0 {
		m.Clients.ChainlinkClient.ChainlinkNodes = m.Common.DockerEnv.Clients
	}
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
