package common

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/onsi/gomega"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	test_env_sol "github.com/smartcontractkit/chainlink-solana/integration-tests/docker/test_env"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/osutil"
	"github.com/smartcontractkit/chainlink/integration-tests/testconfig"

	"golang.org/x/sync/errgroup"

	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/integration-tests/docker/test_env"
)

const (
	ContractsStateFile        = "contracts-chaos-state.json"
	NewRoundCheckTimeout      = 120 * time.Second
	NewSoakRoundsCheckTimeout = 3 * time.Hour
	NewRoundCheckPollInterval = 1 * time.Second
	SourceChangeInterval      = 5 * time.Second
	ChaosAwaitingApply        = 1 * time.Minute
	// ChaosGroupFaulty Group of faulty nodes, even if they fail OCR must work
	ChaosGroupFaulty = "chaosGroupFaulty"
	// ChaosGroupYellow if nodes from that group fail we may not work while some experiments are going
	// but after experiment it must recover
	ChaosGroupYellow = "chaosGroupYellow"
	// ChaosGroupLeftHalf an equal half of all nodes
	ChaosGroupLeftHalf = "chaosGroupLeftHalf"
	// ChaosGroupRightHalf an equal half of all nodes
	ChaosGroupRightHalf = "chaosGroupRightHalf"
	// ChaosGroupOnline a group of nodes that are working
	ChaosGroupOnline = "chaosGroupOnline"
	// UntilStop some chaos experiments doesn't respect absence of duration and got recovered immediately, so we enforce duration
	UntilStop = 666 * time.Hour
)

type Contracts struct {
	BAC       *solclient.AccessController
	RAC       *solclient.AccessController
	OCR2      *solclient.OCRv2
	Store     *solclient.Store
	StoreAuth string
}

func NewOCRv2State(t *testing.T, contracts int, namespacePrefix string, env string, isK8s bool, testConfig *testconfig.TestConfig) (*OCRv2TestState, error) {

	c, err := New(env, isK8s).Default(t, namespacePrefix)
	if err != nil {
		return nil, err
	}
	state := &OCRv2TestState{
		Mu:                 &sync.Mutex{},
		LastRoundTime:      make(map[string]time.Time),
		ContractsNodeSetup: make(map[int]*ContractNodeInfo),
		Common:             c,
		Client:             &solclient.Client{},
		T:                  t,
		L:                  log.Logger,
		TestConfig:         testConfig,
	}
	if state.T != nil {
		state.L = logging.GetTestLogger(state.T)
	}

	state.Client.Config = state.Client.Config.Default()
	for i := 0; i < contracts; i++ {
		state.ContractsNodeSetup[i] = &ContractNodeInfo{}
		state.ContractsNodeSetup[i].BootstrapNodeIdx = 0
		for n := 1; n < state.Common.NodeCount; n++ {
			state.ContractsNodeSetup[i].NodesIdx = append(state.ContractsNodeSetup[i].NodesIdx, n)
		}
	}
	return state, nil
}

type OCRv2TestState struct {
	Mu                 *sync.Mutex
	ChainlinkNodesK8s  []*client.ChainlinkK8sClient
	ChainlinkNodes     []*client.ChainlinkClient
	Contracts          []Contracts
	ContractsNodeSetup map[int]*ContractNodeInfo
	NodeKeysBundle     []client.NodeKeysBundle
	Client             *solclient.Client
	Gauntlet           *gauntlet.SolanaGauntlet
	RoundsFound        int
	LastRoundTime      map[string]time.Time
	err                error
	T                  *testing.T
	Common             *Common
	L                  zerolog.Logger
	TestConfig         *testconfig.TestConfig
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

func (m *OCRv2TestState) LabelChaosGroups() {
	m.LabelChaosGroup(1, 5, ChaosGroupFaulty)
	m.LabelChaosGroup(6, 19, ChaosGroupOnline)
	m.LabelChaosGroup(0, 8, ChaosGroupYellow)
	m.LabelChaosGroup(0, 9, ChaosGroupLeftHalf)
	m.LabelChaosGroup(10, 19, ChaosGroupRightHalf)
}

func (m *OCRv2TestState) DeployCluster(contractsDir string, enableGauntlet bool) {
	if m.Common.IsK8s {
		m.DeployEnv(contractsDir)
	} else {
		env, err := test_env.NewTestEnv()
		require.NoError(m.T, err)
		sol := test_env_sol.NewSolana([]string{env.DockerNetwork.Name})
		err = sol.StartContainer()
		require.NoError(m.T, err)
		m.Common.SolanaUrl = sol.InternalHttpUrl
		b, err := test_env.NewCLTestEnvBuilder().
			WithNonEVM().
			WithTestInstance(m.T).
			WithTestConfig(m.TestConfig).
			WithMockAdapter().
			WithCLNodeConfig(m.Common.DefaultNodeConfig()).
			WithCLNodes(m.Common.NodeCount).
			WithCLNodeOptions(m.Common.NodeOpts...).
			WithStandardCleanup().
			WithTestEnv(env)
		require.NoError(m.T, err)
		env, err = b.Build()
		require.NoError(m.T, err)
		m.Common.DockerEnv = &SolCLClusterTestEnv{
			CLClusterTestEnv: env,
			Sol:              sol,
			Killgrave:        env.MockAdapter,
		}
	}
	m.SetupClients(enableGauntlet)
	m.DeployContracts(contractsDir)
	m.CreateJobs()
}

func (m *OCRv2TestState) LabelChaosGroup(startInstance int, endInstance int, group string) {
	for i := startInstance; i <= endInstance; i++ {
		m.err = m.Common.Env.Client.AddLabel(m.Common.Env.Cfg.Namespace, fmt.Sprintf("instance=%d", i), fmt.Sprintf("%s=1", group))
		require.NoError(m.T, m.err)
	}
}

// UploadProgramBinaries uploads programs binary files to solana-validator container
// currently it's the only way to deploy anything to local solana because ephemeral validator in k8s
// can't expose UDP ports required to copy .so chunks when deploying
func (m *OCRv2TestState) UploadProgramBinaries(contractsDir string) {
	pl, err := m.Common.Env.Client.ListPods(m.Common.Env.Cfg.Namespace, "app=sol")
	require.NoError(m.T, err)
	_, _, _, err = m.Common.Env.Client.CopyToPod(m.Common.Env.Cfg.Namespace, contractsDir, fmt.Sprintf("%s/%s:/programs", m.Common.Env.Cfg.Namespace, pl.Items[0].Name), "sol-val")
	require.NoError(m.T, err)
}

func (m *OCRv2TestState) DeployEnv(contractsDir string) {
	err := m.Common.Env.Run()
	require.NoError(m.T, err)

	m.Common.SolanaUrl = m.Common.Env.URLs[m.Client.Config.Name][0]
	m.UploadProgramBinaries(contractsDir)
}

func (m *OCRv2TestState) NewSolanaClientSetup(networkSettings *solclient.SolNetwork) (*solclient.Client, error) {
	if m.Common.IsK8s {
		networkSettings.URLs = m.Common.Env.URLs[networkSettings.Name]
	} else {
		networkSettings.URLs = []string{
			m.Common.DockerEnv.Sol.ExternalHttpUrl,
			m.Common.DockerEnv.Sol.ExternalWsUrl,
		}
	}
	ec, err := solclient.NewClient(networkSettings)
	if err != nil {
		return nil, err
	}
	m.L.Info().
		Interface("URLs", networkSettings.URLs).
		Msg("Connected Solana client")
	return ec, nil

}

func (m *OCRv2TestState) SetupClients(enableGauntlet bool) {
	// setup direct solana API
	m.Client, m.err = m.NewSolanaClientSetup(m.Client.Config)
	require.NoError(m.T, m.err, "error setting up sol client")

	// setup gauntlet
	if enableGauntlet {
		m.Gauntlet, m.err = gauntlet.NewSolanaGauntlet(fmt.Sprintf("%s/gauntlet", utils.ProjectRoot))
		require.NoError(m.T, m.err, "error setting up gauntlet")
	}

	if m.Common.IsK8s {
		m.ChainlinkNodesK8s, m.err = client.ConnectChainlinkNodes(m.Common.Env)
		require.NoError(m.T, m.err)
	} else {
		m.ChainlinkNodes = m.Common.DockerEnv.ClCluster.NodeAPIs()
	}
}

func (m *OCRv2TestState) initializeNodesInContractsMap() {
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		for _, nodeIndex := range m.ContractsNodeSetup[i].NodesIdx {
			if m.Common.IsK8s {
				m.ContractsNodeSetup[i].NodesK8s = append(m.ContractsNodeSetup[i].NodesK8s, m.ChainlinkNodesK8s[nodeIndex])
			} else {
				m.ContractsNodeSetup[i].Nodes = append(m.ContractsNodeSetup[i].Nodes, m.ChainlinkNodes[nodeIndex])
			}
			m.ContractsNodeSetup[i].NodeKeysBundle = append(m.ContractsNodeSetup[i].NodeKeysBundle, m.NodeKeysBundle[nodeIndex])
		}
		if m.Common.IsK8s {
			m.ContractsNodeSetup[i].BootstrapNodeK8s = m.ChainlinkNodesK8s[m.ContractsNodeSetup[i].BootstrapNodeIdx]
		} else {
			m.ContractsNodeSetup[i].BootstrapNode = m.ChainlinkNodes[m.ContractsNodeSetup[i].BootstrapNodeIdx]
		}
		m.ContractsNodeSetup[i].BootstrapNodeKeysBundle = m.NodeKeysBundle[m.ContractsNodeSetup[i].BootstrapNodeIdx]
	}
}

// DeployContracts deploys contracts
func (m *OCRv2TestState) DeployContracts(contractsDir string) {
	if m.Common.IsK8s {
		m.NodeKeysBundle, m.err = m.Common.CreateNodeKeysBundle(m.GetChainlinkNodes())
	} else {
		m.NodeKeysBundle, m.err = m.Common.CreateNodeKeysBundle(m.Common.DockerEnv.ClCluster.NodeAPIs())
	}
	require.NoError(m.T, m.err)

	// Deploy programs + LINK token via API
	cd, err := solclient.NewContractDeployer(m.Client, nil)
	require.NoError(m.T, err)
	err = cd.LoadPrograms(contractsDir)
	require.NoError(m.T, err)
	if m.Common.IsK8s {
		err = cd.DeployAnchorProgramsRemote(contractsDir, m.Common.Env)
	} else {
		err = cd.DeployAnchorProgramsRemoteDocker(contractsDir, m.Common.DockerEnv.Sol)
	}
	require.NoError(m.T, err)
	cd.RegisterAnchorPrograms()
	require.NoError(m.T, cd.ValidateProgramsDeployed())
	m.Client.LinkToken, err = cd.DeployLinkTokenContract()
	require.NoError(m.T, err)
	err = FundOracles(m.Client, m.NodeKeysBundle, big.NewFloat(1e4)) // TODO: handle if devnet?
	require.NoError(m.T, err)

	m.initializeNodesInContractsMap()

	// Deploy feed instances using solclient or gauntlet
	g := errgroup.Group{}
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		i := i
		g.Go(func() error {
			// use gauntlet if it exists
			if m.Gauntlet != nil {
				return m.DeployFeedWithGauntlet(i)
			}
			return m.DeployFeedWithSolClient(i)
		})
	}
	require.NoError(m.T, g.Wait())
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		m.ContractsNodeSetup[i].OCR2 = m.Contracts[i].OCR2
		m.ContractsNodeSetup[i].Store = m.Contracts[i].Store
	}
}

func (m *OCRv2TestState) DeployFeedWithGauntlet(i int) error {
	gauntletConfig := m.ConfigureGauntletFromState(utils.TestingSecret)
	require.NoError(m.T, m.Gauntlet.SetupNetwork(gauntletConfig))

	err := m.Gauntlet.DeployOCR2()
	require.NoError(m.T, err, "Error deploying OCR")

	// TODO: cleanup duplicate logic
	bundleData := make([]client.NodeKeysBundle, len(m.ContractsNodeSetup[i].NodeKeysBundle))
	copy(bundleData, m.ContractsNodeSetup[i].NodeKeysBundle)

	// We have to sort by on_chain_pub_key for the config digest
	sort.Slice(bundleData, func(i, j int) bool {
		return bundleData[i].OCR2Key.Data.Attributes.OnChainPublicKey < bundleData[j].OCR2Key.Data.Attributes.OnChainPublicKey
	})

	onChainConfig, err := m.GenerateOnChainConfig(bundleData, gauntletConfig["VAULT"], m.Gauntlet.ProposalAddress)
	require.NoError(m.T, err)

	reportingConfig := utils.ReportingPluginConfig{
		AlphaReportInfinite: false,
		AlphaReportPpb:      0,
		AlphaAcceptInfinite: false,
		AlphaAcceptPpb:      0,
		DeltaCNanoseconds:   0,
	}
	offChainConfig := m.GenerateOffChainConfig(
		bundleData,
		m.Gauntlet.ProposalAddress,
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

	payees := m.GeneratePayees(bundleData, gauntletConfig["VAULT"], m.Gauntlet.ProposalAddress)
	proposalAccept := m.GenerateProposalAcceptConfig(m.Gauntlet.ProposalAddress, 2, 1, onChainConfig.Oracles, offChainConfig.OffchainConfig, utils.TestingSecret)

	err = m.Gauntlet.ConfigureOCR2(onChainConfig, offChainConfig, payees, proposalAccept)
	require.NoError(m.T, err)

	// TODO: standardize config generation
	// var nodeCount int
	// if m.Common.IsK8s {
	// 	nodeCount = len(m.ContractsNodeSetup[i].NodesK8s)
	// } else {
	// 	nodeCount = len(m.ContractsNodeSetup[i].Nodes)
	// }
	// ocConfig, err := OffChainConfigParamsFromNodes(nodeCount, m.ContractsNodeSetup[i].NodeKeysBundle)
	// require.NoError(m.T, err)
	// err = ocr2.Configure(ocConfig)
	// require.NoError(m.T, err)

	m.Mu.Lock()
	m.Contracts = append(m.Contracts, Contracts{
		OCR2: &solclient.OCRv2{
			Client:        m.Client,
			State:         solana.MustPublicKeyFromBase58(m.Gauntlet.OcrAddress),
			ProgramWallet: m.Client.ProgramWallets["ocr2-keypair.json"].PublicKey(),
		},
		Store: &solclient.Store{
			Client:        m.Client,
			Store:         solana.MustPublicKeyFromBase58(m.Gauntlet.StoreAddress),
			Feed:          solana.MustPublicKeyFromBase58(m.Gauntlet.FeedAddress),
			ProgramWallet: m.Client.ProgramWallets["store-keypair.json"].PublicKey(),
		},
	})
	m.Mu.Unlock()
	return nil
}

func (m *OCRv2TestState) DeployFeedWithSolClient(i int) error {
	cd, err := solclient.NewContractDeployer(m.Client, m.Client.LinkToken)
	require.NoError(m.T, err)
	err = cd.GenerateAuthorities([]string{"vault", "store"})
	require.NoError(m.T, err)
	bac, err := cd.DeployOCRv2AccessController()
	require.NoError(m.T, err)
	rac, err := cd.DeployOCRv2AccessController()
	require.NoError(m.T, err)
	err = m.Client.WaitForEvents()
	require.NoError(m.T, err)

	store, err := cd.DeployOCRv2Store(bac.Address())
	require.NoError(m.T, err)

	err = cd.CreateFeed("Feed", uint8(18), 10, 1024)
	require.NoError(m.T, err)

	ocr2, err := cd.InitOCR2(bac.Address(), rac.Address())
	require.NoError(m.T, err)

	storeAuth := cd.Accounts.Authorities["store"].PublicKey.String()
	err = bac.AddAccess(storeAuth)
	require.NoError(m.T, err)
	err = m.Client.WaitForEvents()
	require.NoError(m.T, err)

	err = store.SetWriter(storeAuth)
	require.NoError(m.T, err)
	err = store.SetValidatorConfig(80000)
	require.NoError(m.T, err)
	err = m.Client.WaitForEvents()
	require.NoError(m.T, err)

	var nodeCount int
	if m.Common.IsK8s {
		nodeCount = len(m.ContractsNodeSetup[i].NodesK8s)
	} else {
		nodeCount = len(m.ContractsNodeSetup[i].Nodes)
	}
	ocConfig, err := OffChainConfigParamsFromNodes(nodeCount, m.ContractsNodeSetup[i].NodeKeysBundle)
	require.NoError(m.T, err)

	err = ocr2.Configure(ocConfig)
	require.NoError(m.T, err)
	m.Mu.Lock()
	m.Contracts = append(m.Contracts, Contracts{
		BAC:       bac,
		RAC:       rac,
		OCR2:      ocr2,
		Store:     store,
		StoreAuth: storeAuth,
	})
	m.Mu.Unlock()
	return nil
}

// CreateJobs creating OCR jobs and EA stubs
func (m *OCRv2TestState) CreateJobs() {
	var nodes []*client.ChainlinkClient
	var mockInternalUrl string
	if m.Common.IsK8s {
		nodes = m.GetChainlinkNodes()
		mockInternalUrl = m.Common.Env.URLs["qa_mock_adapter_internal"][0]
	} else {
		nodes = m.Common.DockerEnv.ClCluster.NodeAPIs()
		mockInternalUrl = m.Common.DockerEnv.Killgrave.InternalEndpoint
	}
	m.L.Info().Str("Url", mockInternalUrl).Msg("Mock adapter url")
	m.err = m.Common.CreateSolanaChainAndNode(nodes)
	require.NoError(m.T, m.err)
	m.err = CreateBridges(m.ContractsNodeSetup, mockInternalUrl, m.Common.IsK8s)
	require.NoError(m.T, m.err)
	g := errgroup.Group{}
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		i := i
		g.Go(func() error {
			m.err = m.Common.CreateJobsForContract(m.ContractsNodeSetup[i])
			require.NoError(m.T, m.err)
			return nil
		})
	}
	require.NoError(m.T, g.Wait())
}

func (m *OCRv2TestState) ValidateNoRoundsAfter(chaosStartTime time.Time) {
	m.RoundsFound = 0
	for _, c := range m.Contracts {
		m.LastRoundTime[c.OCR2.Address()] = chaosStartTime
	}
	gom := gomega.NewWithT(m.T)
	gom.Consistently(func(g gomega.Gomega) {
		for _, c := range m.Contracts {
			_, timestamp, _, err := c.Store.GetLatestRoundData()
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			roundTime := time.Unix(int64(timestamp), 0)
			g.Expect(roundTime.Before(m.LastRoundTime[c.OCR2.Address()])).Should(gomega.BeTrue())
		}
	}, NewRoundCheckTimeout, NewRoundCheckPollInterval).Should(gomega.Succeed())
}

type Answer struct {
	Answer    uint64
	Timestamp uint64
	Error     error
}

func (m *OCRv2TestState) ValidateRoundsAfter(chaosStartTime time.Time, timeout time.Duration, rounds int) {
	m.RoundsFound = 0
	for _, c := range m.Contracts {
		m.LastRoundTime[c.OCR2.Address()] = chaosStartTime
	}
	roundsFound := 0
	gom := gomega.NewWithT(m.T)
	gom.Eventually(func(g gomega.Gomega) {
		answers := make(map[string]*Answer)
		for _, c := range m.Contracts {
			answer, timestamp, _, err := c.Store.GetLatestRoundData()
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			answers[c.OCR2.Address()] = &Answer{Answer: answer, Timestamp: timestamp, Error: err}
		}
		for ci, a := range answers {
			answerTime := time.Unix(int64(a.Timestamp), 0)
			if answerTime.After(m.LastRoundTime[ci]) {
				m.LastRoundTime[ci] = answerTime
				roundsFound++
				m.L.Debug().Str("Contract", ci).Interface("Answer", a).Int("RoundsFound", roundsFound).Msg("New answer found")
			} else {
				m.L.Debug().Str("Contract", ci).Interface("Answer", a).Msg("Answer has not changed")
			}
		}
		g.Expect(roundsFound).To(gomega.BeNumerically(">=", rounds*len(m.Contracts)))
	}, timeout, NewRoundCheckPollInterval).Should(gomega.Succeed())
}

func (m *OCRv2TestState) GenerateOnChainConfig(nodeKeys []client.NodeKeysBundle, vaultAddress string, proposalId string) (utils.OCR2OnChainConfig, error) {

	var oracles []utils.Operator

	for _, nodeKey := range nodeKeys {
		oracles = append(oracles, utils.Operator{
			Signer:      strings.Replace(nodeKey.OCR2Key.Data.Attributes.OnChainPublicKey, "ocr2on_solana_", "", 1),
			Transmitter: nodeKey.TXKey.Data.Attributes.PublicKey,
			Payee:       vaultAddress,
		})
	}

	return utils.OCR2OnChainConfig{
		Oracles:    oracles,
		F:          1,
		ProposalId: proposalId,
	}, nil
}

func (m *OCRv2TestState) GenerateOffChainConfig(
	nodeKeysBundle []client.NodeKeysBundle,
	proposalId string,
	reportingConfig utils.ReportingPluginConfig,
	deltaProgressNanoseconds int64,
	deltaResendNanoseconds int64,
	deltaRoundNanoseconds int64,
	deltaGraceNanoseconds int64,
	deltaStageNanoseconds int64,
	rMax int,
	maxDurationQueryNanoseconds int64,
	maxDurationObservationNanoseconds int64,
	maxDurationReportNanoseconds int64,
	maxDurationShouldAcceptFinalizedReportNanoseconds int64,
	maxDurationShouldTransmitAcceptedReportNanoseconds int64,
	secret string,

) utils.OCROffChainConfig {

	offchainPublicKeys := make([]string, len(nodeKeysBundle))
	peerIds := make([]string, len(nodeKeysBundle))
	configPublicKeys := make([]string, len(nodeKeysBundle))
	s := make([]int, len(nodeKeysBundle))

	for i := range s {
		s[i] = 1
	}

	for i, bundle := range nodeKeysBundle {
		offchainPublicKeys[i] = strings.Replace(bundle.OCR2Key.Data.Attributes.OffChainPublicKey, "ocr2off_solana_", "", 1)
		peerIds[i] = bundle.PeerID
		configPublicKeys[i] = strings.Replace(bundle.OCR2Key.Data.Attributes.ConfigPublicKey, "ocr2cfg_solana_", "", 1)
	}

	offChainConfig := utils.OCROffChainConfig{
		ProposalId: proposalId,
		OffchainConfig: utils.OffchainConfig{
			DeltaProgressNanoseconds:          deltaProgressNanoseconds,
			DeltaResendNanoseconds:            deltaResendNanoseconds,
			DeltaRoundNanoseconds:             deltaRoundNanoseconds,
			DeltaGraceNanoseconds:             deltaGraceNanoseconds,
			DeltaStageNanoseconds:             deltaStageNanoseconds,
			RMax:                              rMax,
			S:                                 s,
			OffchainPublicKeys:                offchainPublicKeys,
			PeerIds:                           peerIds,
			ConfigPublicKeys:                  configPublicKeys,
			ReportingPluginConfig:             reportingConfig,
			MaxDurationQueryNanoseconds:       maxDurationQueryNanoseconds,
			MaxDurationObservationNanoseconds: maxDurationObservationNanoseconds,
			MaxDurationReportNanoseconds:      maxDurationReportNanoseconds,
			MaxDurationShouldAcceptFinalizedReportNanoseconds:  maxDurationShouldAcceptFinalizedReportNanoseconds,
			MaxDurationShouldTransmitAcceptedReportNanoseconds: maxDurationShouldTransmitAcceptedReportNanoseconds,
		},
		UserSecret: secret,
	}

	return offChainConfig
}

func (m *OCRv2TestState) GeneratePayees(nodeKeys []client.NodeKeysBundle, vaultAddress string, proposalId string) utils.PayeeConfig {
	var operators []utils.Operator
	for _, key := range nodeKeys {
		operators = append(operators, utils.Operator{
			Signer:      strings.Replace(key.OCR2Key.Data.Attributes.OnChainPublicKey, "ocr2on_solana_", "", 1),
			Transmitter: key.TXKey.Data.Attributes.PublicKey,
			Payee:       vaultAddress,
		})
	}

	return utils.PayeeConfig{
		Operators:  operators,
		ProposalId: proposalId,
	}
}

func (m *OCRv2TestState) GenerateProposalAcceptConfig(
	proposalId string,
	version int,
	f int,
	oracles []utils.Operator,
	offChainConfig utils.OffchainConfig,
	randomSecret string,

) utils.ProposalAcceptConfig {
	return utils.ProposalAcceptConfig{
		ProposalId:     proposalId,
		Version:        version,
		F:              f,
		Oracles:        oracles,
		OffchainConfig: offChainConfig,
		RandomSecret:   randomSecret,
	}
}

func (m *OCRv2TestState) ConfigureGauntletFromState(secret string) map[string]string {
	if err := os.Setenv("SECRET", secret); err != nil {
		panic("Error setting SECRET")
	}

	return map[string]string{
		"NODE_URL":                     m.Common.SolanaUrl,
		"PRIVATE_KEY":                  strings.ReplaceAll(fmt.Sprintf("%v", []byte(m.Client.DefaultWallet.PrivateKey)), " ", ","), // base58 privkey -> [#,#,...,#]
		"PROGRAM_ID_OCR2":              m.Client.ProgramWallets["ocr2-keypair.json"].PublicKey().String(),
		"PROGRAM_ID_ACCESS_CONTROLLER": m.Client.ProgramWallets["access_controller-keypair.json"].PublicKey().String(),
		"PROGRAM_ID_STORE":             m.Client.ProgramWallets["store-keypair.json"].PublicKey().String(),
		"LINK":                         m.Client.LinkToken.Address(),
		// unused?
		// "WS_URL":                       wsUrl,
		// "VAULT":                        vault,
	}
}

func (m *OCRv2TestState) ConfigureGauntletFromEnv(secret string) map[string]string {
	err := os.Setenv("SECRET", secret)
	if err != nil {
		panic("Error setting SECRET")
	}
	rpcUrl, exists := os.LookupEnv("RPC_URL")
	if !exists {
		panic("Please define RPC_URL")
	}

	wsUrl, exists := os.LookupEnv("WS_URL")
	if !exists {
		panic("Please define WS_URL")
	}
	privateKey, exists := os.LookupEnv("PRIVATE_KEY")
	if !exists {
		panic("Please define PRIVATE_KEY")
	}
	programIdOCR2, exists := os.LookupEnv("PROGRAM_ID_OCR2")
	if !exists {
		panic("Please define PROGRAM_ID_OCR2")
	}

	programIdAccessController, exists := os.LookupEnv("PROGRAM_ID_ACCESS_CONTROLLER")
	if !exists {
		panic("Please define PROGRAM_ID_ACCESS_CONTROLLER")
	}

	programIdStore, exists := os.LookupEnv("PROGRAM_ID_STORE")
	if !exists {
		panic("Please define PROGRAM_ID_STORE")
	}

	linkToken, exists := os.LookupEnv("LINK_TOKEN")
	if !exists {
		panic("Please define LINK_TOKEN")
	}

	vault, exists := os.LookupEnv("VAULT_ADDRESS")
	if !exists {
		panic("Please define VAULT_ADDRESS")
	}

	return map[string]string{
		"NODE_URL":                     rpcUrl,
		"WS_URL":                       wsUrl,
		"PRIVATE_KEY":                  privateKey,
		"PROGRAM_ID_OCR2":              programIdOCR2,
		"PROGRAM_ID_ACCESS_CONTROLLER": programIdAccessController,
		"PROGRAM_ID_STORE":             programIdStore,
		"LINK":                         linkToken,
		"VAULT":                        vault,
	}

}

// GauntletEnvToRemoteRunner Setup the environment variables that will be needed inside the remote runner
func (m *OCRv2TestState) GauntletEnvToRemoteRunner() {
	osutil.SetupEnvVarsForRemoteRunner([]string{
		"RPC_URL",
		"WS_URL",
		"PRIVATE_KEY",
		"PROGRAM_ID_OCR2",
		"PROGRAM_ID_ACCESS_CONTROLLER",
		"PROGRAM_ID_STORE",
		"LINK_TOKEN",
		"VAULT_ADDRESS",
	})
}

func (m *OCRv2TestState) GetChainlinkNodes() []*client.ChainlinkClient {
	// retrieve client from K8s client
	chainlinkNodes := []*client.ChainlinkClient{}
	for i := range m.ChainlinkNodesK8s {
		chainlinkNodes = append(chainlinkNodes, m.ChainlinkNodesK8s[i].ChainlinkClient)
	}
	return chainlinkNodes
}
