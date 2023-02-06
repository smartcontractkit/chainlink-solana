package common

import (
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-env/environment"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"

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

func NewOCRv2State(t *testing.T, contracts int) *OCRv2TestState {
	state := &OCRv2TestState{
		Mu:                 &sync.Mutex{},
		LastRoundTime:      make(map[string]time.Time),
		ContractsNodeSetup: make(map[int]*ContractNodeInfo),
		Common:             New().Default(t),
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
	Env                *environment.Environment
	ChainlinkNodes     []*client.Chainlink
	ContractDeployer   *solclient.ContractDeployer
	LinkToken          *solclient.LinkToken
	Contracts          []Contracts
	ContractsNodeSetup map[int]*ContractNodeInfo
	NodeKeysBundle     []client.NodeKeysBundle
	MockServer         *ctfClient.MockserverClient
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
	if m.Env.WillUseRemoteRunner() {
		return
	}
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
	if m.Common.Env.WillUseRemoteRunner() {
		return
	}

	m.Common.SolanaUrl = m.Common.Env.URLs[m.Client.Config.Name][0]
	m.UploadProgramBinaries(contractsDir)
}

func (m *OCRv2TestState) NewSolanaClientSetup(networkSettings *solclient.SolNetwork) func(*environment.Environment) (*solclient.Client, error) {
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
	m.Client, m.err = m.NewSolanaClientSetup(m.Client.Config)(m.Common.Env)
	require.NoError(m.T, m.err)
	m.MockServer, m.err = ctfClient.ConnectMockServer(m.Common.Env)
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
	m.NodeKeysBundle, m.err = CreateNodeKeysBundle(m.ChainlinkNodes)
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
	m.err = m.MockServer.SetValuePath("/juels", 1)
	require.NoError(m.T, m.err)
	m.err = m.Common.CreateSolanaChainAndNode(m.ChainlinkNodes)
	require.NoError(m.T, m.err)
	m.err = CreateBridges(m.ContractsNodeSetup, m.MockServer)
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
