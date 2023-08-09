package common

import (
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-env/environment"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-testing-framework/utils"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"golang.org/x/sync/errgroup"
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

type OCR2OnChainConfig struct {
	Oracles    []Operator `json:"oracles"`
	F          int        `json:"f"`
	ProposalId string     `json:"proposalId"`
}

type OffchainConfig struct {
	DeltaProgressNanoseconds                           int64                 `json:"deltaProgressNanoseconds"`
	DeltaResendNanoseconds                             int64                 `json:"deltaResendNanoseconds"`
	DeltaRoundNanoseconds                              int64                 `json:"deltaRoundNanoseconds"`
	DeltaGraceNanoseconds                              int64                 `json:"deltaGraceNanoseconds"`
	DeltaStageNanoseconds                              int64                 `json:"deltaStageNanoseconds"`
	RMax                                               int                   `json:"rMax"`
	S                                                  []int                 `json:"s"`
	OffchainPublicKeys                                 []string              `json:"offchainPublicKeys"`
	PeerIds                                            []string              `json:"peerIds"`
	ReportingPluginConfig                              ReportingPluginConfig `json:"reportingPluginConfig"`
	MaxDurationQueryNanoseconds                        int64                 `json:"maxDurationQueryNanoseconds"`
	MaxDurationObservationNanoseconds                  int64                 `json:"maxDurationObservationNanoseconds"`
	MaxDurationReportNanoseconds                       int64                 `json:"maxDurationReportNanoseconds"`
	MaxDurationShouldAcceptFinalizedReportNanoseconds  int64                 `json:"maxDurationShouldAcceptFinalizedReportNanoseconds"`
	MaxDurationShouldTransmitAcceptedReportNanoseconds int64                 `json:"maxDurationShouldTransmitAcceptedReportNanoseconds"`
	ConfigPublicKeys                                   []string              `json:"configPublicKeys"`
}

type ReportingPluginConfig struct {
	AlphaReportInfinite bool `json:"alphaReportInfinite"`
	AlphaReportPpb      int  `json:"alphaReportPpb"`
	AlphaAcceptInfinite bool `json:"alphaAcceptInfinite"`
	AlphaAcceptPpb      int  `json:"alphaAcceptPpb"`
	DeltaCNanoseconds   int  `json:"deltaCNanoseconds"`
}

// TODO - Decouple all OCR2 config structs to be reusable between chains
type OCROffChainConfig struct {
	ProposalId     string         `json:"proposalId"`
	OffchainConfig OffchainConfig `json:"offchainConfig"`
	UserSecret     string         `json:"userSecret"`
}

type Operator struct {
	Signer      string `json:"signer"`
	Transmitter string `json:"transmitter"`
	Payee       string `json:"payee"`
}

type PayeeConfig struct {
	Operators  []Operator `json:"operators"`
	ProposalId string     `json:"proposalId"`
}

type ProposalAcceptConfig struct {
	ProposalId     string         `json:"proposalId"`
	Version        int            `json:"version"`
	F              int            `json:"f"`
	Oracles        []Operator     `json:"oracles"`
	OffchainConfig OffchainConfig `json:"offchainConfig"`
	RandomSecret   string         `json:"randomSecret"`
}

func NewOCRv2State(t *testing.T, contracts int, namespacePrefix string, env string) *OCRv2TestState {

	state := &OCRv2TestState{
		Mu:                 &sync.Mutex{},
		LastRoundTime:      make(map[string]time.Time),
		ContractsNodeSetup: make(map[int]*ContractNodeInfo),
		Common:             New(env).Default(t, namespacePrefix),
		Client:             &solclient.Client{},
		T:                  t,
	}

	state.Client.Config = state.Client.Config.Default()
	for i := 0; i < contracts; i++ {
		state.ContractsNodeSetup[i] = &ContractNodeInfo{}
		state.ContractsNodeSetup[i].BootstrapNodeIdx = 0
		for n := 1; n < state.Common.NodeCount; n++ {
			state.ContractsNodeSetup[i].NodesIdx = append(state.ContractsNodeSetup[i].NodesIdx, n)
		}
	}
	return state
}

type OCRv2TestState struct {
	Mu                 *sync.Mutex
	ChainlinkNodes     []*client.ChainlinkK8sClient
	ContractDeployer   *solclient.ContractDeployer
	LinkToken          *solclient.LinkToken
	Contracts          []Contracts
	ContractsNodeSetup map[int]*ContractNodeInfo
	NodeKeysBundle     []client.NodeKeysBundle
	Client             *solclient.Client
	RoundsFound        int
	LastRoundTime      map[string]time.Time
	err                error
	T                  *testing.T
	Common             *Common
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

func (m *OCRv2TestState) DeployCluster(contractsDir string) {
	m.DeployEnv(contractsDir)
	m.SetupClients()
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

func (m *OCRv2TestState) NewSolanaClientSetup(networkSettings *solclient.SolNetwork) func(*environment.Environment) (*solclient.Client, error) {
	return func(env *environment.Environment) (*solclient.Client, error) {
		l := utils.GetTestLogger(m.T)
		networkSettings.URLs = env.URLs[networkSettings.Name]
		ec, err := solclient.NewClient(networkSettings)
		if err != nil {
			return nil, err
		}
		l.Info().
			Interface("URLs", networkSettings.URLs).
			Msg("Connected Solana client")
		return ec, nil
	}
}

func (m *OCRv2TestState) SetupClients() {
	m.Client, m.err = m.NewSolanaClientSetup(m.Client.Config)(m.Common.Env)
	require.NoError(m.T, m.err)
	m.ChainlinkNodes, m.err = client.ConnectChainlinkNodes(m.Common.Env)
	require.NoError(m.T, m.err)
}

func (m *OCRv2TestState) initializeNodesInContractsMap() {
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		for _, nodeIndex := range m.ContractsNodeSetup[i].NodesIdx {
			m.ContractsNodeSetup[i].Nodes = append(m.ContractsNodeSetup[i].Nodes, m.ChainlinkNodes[nodeIndex])
			m.ContractsNodeSetup[i].NodeKeysBundle = append(m.ContractsNodeSetup[i].NodeKeysBundle, m.NodeKeysBundle[nodeIndex])
		}
		m.ContractsNodeSetup[i].BootstrapNode = m.ChainlinkNodes[m.ContractsNodeSetup[i].BootstrapNodeIdx]
		m.ContractsNodeSetup[i].BootstrapNodeKeysBundle = m.NodeKeysBundle[m.ContractsNodeSetup[i].BootstrapNodeIdx]
	}
}

// DeployContracts deploys contracts
func (m *OCRv2TestState) DeployContracts(contractsDir string) {
	m.NodeKeysBundle, m.err = m.Common.CreateNodeKeysBundle(m.GetChainlinkNodes())
	require.NoError(m.T, m.err)
	cd, err := solclient.NewContractDeployer(m.Client, m.Common.Env, nil)
	require.NoError(m.T, err)
	err = cd.LoadPrograms(contractsDir)
	require.NoError(m.T, err)
	err = cd.DeployAnchorProgramsRemote(contractsDir)
	require.NoError(m.T, err)
	cd.RegisterAnchorPrograms()
	m.Client.LinkToken, err = cd.DeployLinkTokenContract()
	require.NoError(m.T, err)
	err = FundOracles(m.Client, m.NodeKeysBundle, big.NewFloat(1e4))
	require.NoError(m.T, err)

	m.initializeNodesInContractsMap()
	g := errgroup.Group{}
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		i := i
		g.Go(func() error {
			cd, err := solclient.NewContractDeployer(m.Client, m.Common.Env, m.Client.LinkToken)
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

			ocConfig, err := OffChainConfigParamsFromNodes(m.ContractsNodeSetup[i].Nodes, m.ContractsNodeSetup[i].NodeKeysBundle)
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
		})
	}
	require.NoError(m.T, g.Wait())
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		m.ContractsNodeSetup[i].OCR2 = m.Contracts[i].OCR2
		m.ContractsNodeSetup[i].Store = m.Contracts[i].Store
	}
}

// CreateJobs creating OCR jobs and EA stubs
func (m *OCRv2TestState) CreateJobs() {

	m.err = m.Common.CreateSolanaChainAndNode(m.GetChainlinkNodes())
	require.NoError(m.T, m.err)
	m.err = CreateBridges(m.ContractsNodeSetup, m.Common.Env.URLs["qa_mock_adapter_internal"][0])
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
	l := utils.GetTestLogger(m.T)
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
				l.Debug().Str("Contract", ci).Interface("Answer", a).Int("RoundsFound", roundsFound).Msg("New answer found")
			} else {
				l.Debug().Str("Contract", ci).Interface("Answer", a).Msg("Answer haven't changed")
			}
		}
		g.Expect(roundsFound).To(gomega.BeNumerically(">=", rounds*len(m.Contracts)))
	}, timeout, NewRoundCheckPollInterval).Should(gomega.Succeed())
}

func (m *OCRv2TestState) GenerateOnChainConfig(nodeKeys []client.NodeKeysBundle, vaultAddress string, proposalId string) (OCR2OnChainConfig, error) {

	var oracles []Operator

	for _, nodeKey := range nodeKeys {
		oracles = append(oracles, Operator{
			Signer:      strings.Replace(nodeKey.OCR2Key.Data.Attributes.OnChainPublicKey, "ocr2on_solana_", "", 1),
			Transmitter: nodeKey.TXKey.Data.Attributes.PublicKey,
			Payee:       vaultAddress,
		})
	}

	return OCR2OnChainConfig{
		Oracles:    oracles,
		F:          1,
		ProposalId: proposalId,
	}, nil
}

func (m *OCRv2TestState) GenerateOffChainConfig(
	nodeKeysBundle []client.NodeKeysBundle,
	proposalId string,
	reportingConfig ReportingPluginConfig,
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

) OCROffChainConfig {

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

	offChainConfig := OCROffChainConfig{
		ProposalId: proposalId,
		OffchainConfig: OffchainConfig{
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

func (m *OCRv2TestState) GeneratePayees(nodeKeys []client.NodeKeysBundle, vaultAddress string, proposalId string) PayeeConfig {
	var operators []Operator
	for _, key := range nodeKeys {
		operators = append(operators, Operator{
			Signer:      strings.Replace(key.OCR2Key.Data.Attributes.OnChainPublicKey, "ocr2on_solana_", "", 1),
			Transmitter: key.TXKey.Data.Attributes.PublicKey,
			Payee:       vaultAddress,
		})
	}

	return PayeeConfig{
		Operators:  operators,
		ProposalId: proposalId,
	}
}

func (m *OCRv2TestState) GenerateProposalAcceptConfig(
	proposalId string,
	version int,
	f int,
	oracles []Operator,
	offChainConfig OffchainConfig,
	randomSecret string,

) ProposalAcceptConfig {
	return ProposalAcceptConfig{
		ProposalId:     proposalId,
		Version:        version,
		F:              f,
		Oracles:        oracles,
		OffchainConfig: offChainConfig,
		RandomSecret:   randomSecret,
	}
}

func (m *OCRv2TestState) ConfigureGauntlet(secret string) map[string]string {
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
	utils.SetupEnvVarsForRemoteRunner([]string{
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
	for i := range m.ChainlinkNodes {
		chainlinkNodes = append(chainlinkNodes, m.ChainlinkNodes[i].ChainlinkClient)
	}
	return chainlinkNodes
}
