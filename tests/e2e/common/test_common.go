package common

//revive:disable:dot-imports
import (
	"fmt"
	"github.com/stretchr/testify/require"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-env/pkg/helm/chainlink"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/sol"

	"github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-env/environment"
	mockservercfg "github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver-cfg"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/solclient"

	ctfClient "github.com/smartcontractkit/chainlink-testing-framework/client"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"golang.org/x/sync/errgroup"
)

const (
	ContractsStateFile        = "contracts-chaos-state.json"
	NewRoundCheckTimeout      = 120 * time.Second
	NewSoakRoundsCheckTimeout = 6 * time.Hour
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

func NewOCRv2State(t *testing.T, contracts int, nodes int) *OCRv2TestState {
	state := &OCRv2TestState{
		Mu:                 &sync.Mutex{},
		LastRoundTime:      make(map[string]time.Time),
		ContractsNodeSetup: make(map[int]*ContractNodeInfo),
		T:                  t,
	}
	for i := 0; i < contracts; i++ {
		state.ContractsNodeSetup[i] = &ContractNodeInfo{}
		state.ContractsNodeSetup[i].BootstrapNodeIdx = 0
		for n := 1; n < nodes; n++ {
			state.ContractsNodeSetup[i].NodesIdx = append(state.ContractsNodeSetup[i].NodesIdx, n)
		}
	}
	return state
}

type OCRv2TestState struct {
	Mu                 *sync.Mutex
	Env                *environment.Environment
	ChainlinkNodes     []*client.Chainlink
	ContractDeployer   *solclient.ContractDeployer
	LinkToken          *solclient.LinkToken
	Contracts          []Contracts
	ContractsNodeSetup map[int]*ContractNodeInfo
	NodeKeysBundle     []NodeKeysBundle
	MockServer         *ctfClient.MockserverClient
	c                  *solclient.Client
	RoundsFound        int
	LastRoundTime      map[string]time.Time
	err                error
	T                  *testing.T
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

func (m *OCRv2TestState) DeployCluster(nodes int, stateful bool, contractsDir string) {
	m.DeployEnv(nodes, stateful, contractsDir)
	m.SetupClients()
	m.DeployContracts(contractsDir)
	m.CreateJobs()
}

func (m *OCRv2TestState) LabelChaosGroup(startInstance int, endInstance int, group string) {
	for i := startInstance; i <= endInstance; i++ {
		m.err = m.Env.Client.AddLabel(m.Env.Cfg.Namespace, fmt.Sprintf("instance=%d", i), fmt.Sprintf("%s=1", group))
		require.NoError(m.T, m.err)
	}
}

// UploadProgramBinaries uploads programs binary files to solana-validator container
// currently it's the only way to deploy anything to local solana because ephemeral validator in k8s
// can't expose UDP ports required to copy .so chunks when deploying
func (m *OCRv2TestState) UploadProgramBinaries(contractsDir string) {
	pl, err := m.Env.Client.ListPods(m.Env.Cfg.Namespace, "app=sol")
	require.NoError(m.T, err)
	_, _, _, err = m.Env.Client.CopyToPod(m.Env.Cfg.Namespace, contractsDir, fmt.Sprintf("%s/%s:/programs", m.Env.Cfg.Namespace, pl.Items[0].Name), "sol-val")
	require.NoError(m.T, err)
}

func (m *OCRv2TestState) DeployEnv(nodes int, stateful bool, contractsDir string) {
	m.Env = environment.New(&environment.Config{
		NamespacePrefix: "chainlink-test-sol",
		TTL:             3 * time.Hour,
	}).
		AddHelm(mockservercfg.New(nil)).
		AddHelm(mockserver.New(nil)).
		AddHelm(sol.New(nil)).
		// TODO: Convert to utilize TOML config when available
		AddHelm(chainlink.NewVersioned(0, "0.0.11", map[string]interface{}{
			"replicas": nodes,
			"env": map[string]interface{}{
				"SOLANA_ENABLED":              "true",
				"EVM_ENABLED":                 "false",
				"EVM_RPC_ENABLED":             "false",
				"CHAINLINK_DEV":               "false",
				"FEATURE_OFFCHAIN_REPORTING2": "true",
				"feature_offchain_reporting":  "false",
				"P2P_NETWORKING_STACK":        "V2",
				"P2PV2_LISTEN_ADDRESSES":      "0.0.0.0:6690",
				"P2PV2_DELTA_DIAL":            "5s",
				"P2PV2_DELTA_RECONCILE":       "5s",
				"p2p_listen_port":             "0",
			},
		}))
	err := m.Env.Run()
	require.NoError(m.T, err)
	m.UploadProgramBinaries(contractsDir)
}

func NewSolanaClientSetup(networkSettings *solclient.SolNetwork) func(*environment.Environment) (*solclient.Client, error) {
	return func(env *environment.Environment) (*solclient.Client, error) {
		networkSettings.URLs = env.URLs[networkSettings.Name]
		ec, err := solclient.NewClient(networkSettings)
		if err != nil {
			return nil, err
		}
		log.Info().
			Interface("URLs", networkSettings.URLs).
			Msg("Connected Solana client")
		return ec, nil
	}
}

func (m *OCRv2TestState) SetupClients() {
	m.c, m.err = NewSolanaClientSetup(&solclient.SolNetwork{
		Name:              "sol",
		Type:              "solana",
		ContractsDeployed: false,
		PrivateKeys: []string{
			"57qbvFjTChfNwQxqkFZwjHp7xYoPZa7f9ow6GA59msfCH1g6onSjKUTrrLp4w1nAwbwQuit8YgJJ2AwT9BSwownC",
			"2tye1GyG7wwTUS2T8puXSErDyzQcBxpgwRN5R2MMy5osJKjQF6ZoeYTTpeHaAxpuiE1G4Pnq4sTa4YCWx3RcXb4Y",
			"5aRBAnU3NBymRyMtrRjPLZ3erZNgTZBhEHszsXF8kTwbGLz8q5FYgKicJ7AFifrFitvJB2NS5jbyQohSJtvkgPER",
			"2MYG6HKpMuGEo3qErj4pAF2Gney6Yb6jgjTc6TZCuu7fiLAVQekTd3HbsT9ienzGHpKwA7Ekj2TGuMHPUB6EHJ8P",
		},
		URLs: []string{
			"http://localhost:8899",
			"ws://localhost:8900",
		},
	})(m.Env)
	require.NoError(m.T, m.err)
	m.MockServer, m.err = ctfClient.ConnectMockServer(m.Env)
	require.NoError(m.T, m.err)
	m.ChainlinkNodes, m.err = client.ConnectChainlinkNodes(m.Env)
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
	m.NodeKeysBundle, m.err = CreateNodeKeysBundle(m.ChainlinkNodes)
	require.NoError(m.T, m.err)
	cd, err := solclient.NewContractDeployer(m.c, m.Env, nil)
	require.NoError(m.T, err)
	err = cd.LoadPrograms(contractsDir)
	require.NoError(m.T, err)
	err = cd.DeployAnchorProgramsRemote(contractsDir)
	require.NoError(m.T, err)
	cd.RegisterAnchorPrograms()
	m.LinkToken, err = cd.DeployLinkTokenContract()
	require.NoError(m.T, err)
	err = FundOracles(m.c, m.NodeKeysBundle, big.NewFloat(1e4))
	require.NoError(m.T, err)

	m.initializeNodesInContractsMap()
	g := errgroup.Group{}
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		i := i
		g.Go(func() error {
			cd, err := solclient.NewContractDeployer(m.c, m.Env, m.LinkToken)
			require.NoError(m.T, err)
			err = cd.GenerateAuthorities([]string{"vault", "store"})
			require.NoError(m.T, err)
			bac, err := cd.DeployOCRv2AccessController()
			require.NoError(m.T, err)
			rac, err := cd.DeployOCRv2AccessController()
			require.NoError(m.T, err)
			err = m.c.WaitForEvents()
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
			err = m.c.WaitForEvents()
			require.NoError(m.T, err)

			err = store.SetWriter(storeAuth)
			require.NoError(m.T, err)
			err = store.SetValidatorConfig(80000)
			require.NoError(m.T, err)
			err = m.c.WaitForEvents()
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
	m.err = m.MockServer.SetValuePath("/juels", 1)
	require.NoError(m.T, m.err)
	m.err = CreateSolanaChainAndNode(m.ChainlinkNodes)
	require.NoError(m.T, m.err)
	m.err = CreateBridges(m.ContractsNodeSetup, m.MockServer)
	require.NoError(m.T, m.err)
	g := errgroup.Group{}
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		i := i
		g.Go(func() error {
			m.err = CreateJobsForContract(m.ContractsNodeSetup[i])
			require.NoError(m.T, m.err)
			return nil
		})
	}
	require.NoError(m.T, g.Wait())
}

func (m *OCRv2TestState) SetAllAdapterResponsesToTheSameValue(response int) {
	for i := 0; i < len(m.ContractsNodeSetup); i++ {
		for _, node := range m.ContractsNodeSetup[i].Nodes {
			nodeContractPairID, err := BuildNodeContractPairID(node, m.ContractsNodeSetup[i].OCR2.Address())
			require.NoError(m.T, err)
			path := fmt.Sprintf("/%s", nodeContractPairID)
			m.err = m.MockServer.SetValuePath(path, response)
			require.NoError(m.T, m.err)
		}
	}
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
				log.Debug().Str("Contract", ci).Interface("Answer", a).Int("RoundsFound", roundsFound).Msg("New answer found")
			} else {
				log.Debug().Str("Contract", ci).Interface("Answer", a).Msg("Answer haven't changed")
			}
		}
		g.Expect(roundsFound).To(gomega.BeNumerically(">=", rounds*len(m.Contracts)))
	}, timeout, NewRoundCheckPollInterval).Should(gomega.Succeed())
}
