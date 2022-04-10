package common

//revive:disable:dot-imports
import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/solclient"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
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

func NewOCRv2State(contracts int, nodes int) *OCRv2TestState {
	return &OCRv2TestState{
		Mu:            &sync.Mutex{},
		ContractsNum:  contracts,
		LastRoundTime: map[string]time.Time{},
	}
}

type OCRv2TestState struct {
	Mu               *sync.Mutex
	Env              *environment.Environment
	ChainlinkNodes   []client.Chainlink
	ContractDeployer *solclient.ContractDeployer
	LinkToken        *solclient.LinkToken
	ContractsNum     int
	Contracts        []Contracts
	OffChainConfig   contracts.OffChainAggregatorV2Config
	NodeKeysBundle   []NodeKeysBundle
	MockServer       *client.MockserverClient
	Networks         *client.Networks
	RoundsFound      int
	LastRoundTime    map[string]time.Time
	err              error
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
		m.err = m.Env.AddLabel(fmt.Sprintf("instance=%d", i), fmt.Sprintf("%s=1", group))
		Expect(m.err).ShouldNot(HaveOccurred())
	}
}

// UploadProgramBinaries uploads programs binary files to solana-validator container
// currently it's the only way to deploy anything to local solana because ephemeral validator in k8s
// can't expose UDP ports required to copy .so chunks when deploying
func (m *OCRv2TestState) UploadProgramBinaries(contractsDir string) {
	connections := m.Env.Charts.Connections("solana-validator")
	cc, err := connections.Load("sol", "0", "sol-val")
	Expect(err).ShouldNot(HaveOccurred())
	_, _, _, err = m.Env.Charts["solana-validator"].CopyToPod(contractsDir, fmt.Sprintf("%s/%s:/programs", m.Env.Namespace, cc.PodName), "sol-val")
	Expect(err).ShouldNot(HaveOccurred())
}

func (m *OCRv2TestState) DeployEnv(nodes int, stateful bool, contractsDir string) {
	m.Env, m.err = environment.DeployOrLoadEnvironment(
		solclient.NewChainlinkSolOCRv2(nodes, stateful),
		tools.ChartsRoot,
	)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.Env.ConnectAll()
	Expect(m.err).ShouldNot(HaveOccurred())
	m.UploadProgramBinaries(contractsDir)
}

func (m *OCRv2TestState) SetupClients() {
	networkRegistry := client.NewDefaultNetworkRegistry()
	networkRegistry.RegisterNetwork(
		"solana",
		solclient.ClientInitFunc(),
		solclient.ClientURLSFunc(),
	)
	m.Networks, m.err = networkRegistry.GetNetworks(m.Env)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.MockServer, m.err = client.ConnectMockServer(m.Env)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.ChainlinkNodes, m.err = client.ConnectChainlinkNodes(m.Env)
	Expect(m.err).ShouldNot(HaveOccurred())
}

func (m *OCRv2TestState) DeployContracts(contractsDir string) {
	m.OffChainConfig, m.NodeKeysBundle, m.err = DefaultOffChainConfigParamsFromNodes(m.ChainlinkNodes)
	Expect(m.err).ShouldNot(HaveOccurred())
	cd, err := solclient.NewContractDeployer(m.Networks.Default, m.Env, nil)
	Expect(err).ShouldNot(HaveOccurred())
	err = cd.LoadPrograms(contractsDir)
	Expect(err).ShouldNot(HaveOccurred())
	err = cd.DeployAnchorProgramsRemote(contractsDir)
	Expect(err).ShouldNot(HaveOccurred())
	cd.RegisterAnchorPrograms()
	m.LinkToken, err = cd.DeployLinkTokenContract()
	Expect(err).ShouldNot(HaveOccurred())
	err = FundOracles(m.Networks.Default, m.NodeKeysBundle, big.NewFloat(1e4))
	Expect(err).ShouldNot(HaveOccurred())

	wg := errgroup.Group{}
	for i := 0; i < m.ContractsNum; i++ {
		wg.Go(func() error {
			defer ginkgo.GinkgoRecover()
			cd, err := solclient.NewContractDeployer(m.Networks.Default, m.Env, m.LinkToken)
			Expect(err).ShouldNot(HaveOccurred())
			err = cd.GenerateAuthorities([]string{"vault", "store"})
			Expect(err).ShouldNot(HaveOccurred())
			bac, err := cd.DeployOCRv2AccessController()
			Expect(err).ShouldNot(HaveOccurred())
			rac, err := cd.DeployOCRv2AccessController()
			Expect(err).ShouldNot(HaveOccurred())
			err = m.Networks.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			store, err := cd.DeployOCRv2Store(bac.Address())
			Expect(err).ShouldNot(HaveOccurred())

			err = cd.CreateFeed("Feed", uint8(18), 10, 1024)
			Expect(err).ShouldNot(HaveOccurred())

			ocr2, err := cd.InitOCR2(bac.Address(), rac.Address())
			Expect(err).ShouldNot(HaveOccurred())

			storeAuth := cd.Accounts.Authorities["store"].PublicKey.String()
			err = bac.AddAccess(storeAuth)
			Expect(err).ShouldNot(HaveOccurred())
			err = m.Networks.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			err = store.SetWriter(storeAuth)
			Expect(err).ShouldNot(HaveOccurred())
			err = store.SetValidatorConfig(80000)
			Expect(err).ShouldNot(HaveOccurred())
			err = m.Networks.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			err = ocr2.Configure(m.OffChainConfig)
			Expect(err).ShouldNot(HaveOccurred())
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
	Expect(wg.Wait()).ShouldNot(HaveOccurred())
}

type BridgeSources struct {
	ObservaionSource string
	JuelsSource      string
}

func (m *OCRv2TestState) createBridges() []BridgeSources {
	bs := make([]BridgeSources, 0)
	for nIdx, n := range m.ChainlinkNodes {
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        "variable",
			URL:         fmt.Sprintf("%s/node%d", m.MockServer.Config.ClusterURL, nIdx),
			RequestData: "{}",
		}
		observationSource := client.ObservationSourceSpecBridge(sourceValueBridge)
		err := n.CreateBridge(&sourceValueBridge)
		Expect(err).ShouldNot(HaveOccurred())

		juelsBridge := client.BridgeTypeAttributes{
			Name:        "juels",
			URL:         fmt.Sprintf("%s/juels", m.MockServer.Config.ClusterURL),
			RequestData: "{}",
		}
		juelsSource := client.ObservationSourceSpecBridge(juelsBridge)
		err = n.CreateBridge(&juelsBridge)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = n.CreateSolanaChain(&client.SolanaChainAttributes{ChainID: "localnet"})
		Expect(err).ShouldNot(HaveOccurred())
		_, err = n.CreateSolanaNode(&client.SolanaNodeAttributes{
			Name:          "solana",
			SolanaChainID: "localnet",
			SolanaURL:     "http://sol:8899",
		})
		Expect(err).ShouldNot(HaveOccurred())
		bs = append(bs, BridgeSources{ObservaionSource: observationSource, JuelsSource: juelsSource})
	}
	return bs
}

func (m *OCRv2TestState) createJobs() error {
	bridges := m.createBridges()
	wg := errgroup.Group{}
	for _, contract := range m.Contracts {
		contract := contract
		wg.Go(func() error {
			relayConfig := map[string]string{
				"nodeEndpointHTTP": "http://sol:8899",
				"ocr2ProgramID":    contract.OCR2.ProgramAddress(),
				"transmissionsID":  contract.Store.TransmissionsAddress(),
				"storeProgramID":   contract.Store.ProgramAddress(),
				"chainID":          "localnet",
			}
			bootstrapPeers := []client.P2PData{
				{
					RemoteIP:   m.ChainlinkNodes[0].RemoteIP(),
					RemotePort: "6690",
					PeerID:     m.NodeKeysBundle[0].PeerID,
				},
			}
			for nIdx, n := range m.ChainlinkNodes {
				jobType := "offchainreporting2"
				if nIdx == 0 {
					jobType = "bootstrap"
				}
				jobSpec := &client.OCR2TaskJobSpec{
					Name:                  fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
					JobType:               jobType,
					ContractID:            contract.OCR2.Address(),
					Relay:                 ChainName,
					RelayConfig:           relayConfig,
					PluginType:            "median",
					P2PPeerID:             m.NodeKeysBundle[nIdx].PeerID,
					P2PBootstrapPeers:     bootstrapPeers,
					OCRKeyBundleID:        m.NodeKeysBundle[nIdx].OCR2Key.Data.ID,
					TransmitterID:         m.NodeKeysBundle[nIdx].TXKey.Data.ID,
					ObservationSource:     bridges[nIdx].ObservaionSource,
					JuelsPerFeeCoinSource: bridges[nIdx].JuelsSource,
					TrackerPollInterval:   10 * time.Second, // faster config checking
				}
				_, err := n.CreateJob(jobSpec)
				Expect(err).ShouldNot(HaveOccurred())
			}
			return nil
		})
	}
	return wg.Wait()
}

func (m *OCRv2TestState) SetAllAdapterResponsesToTheSameValue(response int) {
	for i := range m.ChainlinkNodes {
		path := fmt.Sprintf("/node%d", i)
		_ = m.MockServer.SetValuePath(path, response)
	}
}

func (m *OCRv2TestState) SetAllAdapterResponsesToDifferentValues(responses []int) {
	Expect(len(responses)).Should(BeNumerically("==", len(m.ChainlinkNodes)))
	for i := range m.ChainlinkNodes {
		_ = m.MockServer.SetValuePath(fmt.Sprintf("/node%d", i), responses[i])
	}
}

func (m *OCRv2TestState) CreateJobs() {
	m.err = m.MockServer.SetValuePath("/juels", 1)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.createJobs()
	Expect(m.err).ShouldNot(HaveOccurred())
}

func (m *OCRv2TestState) ValidateNoRoundsAfter(chaosStartTime time.Time) {
	m.RoundsFound = 0
	for _, c := range m.Contracts {
		m.LastRoundTime[c.OCR2.Address()] = chaosStartTime
	}
	Consistently(func(g Gomega) {
		for _, c := range m.Contracts {
			_, timestamp, _, err := c.Store.GetLatestRoundData()
			g.Expect(err).ShouldNot(HaveOccurred())
			roundTime := time.Unix(int64(timestamp), 0)
			g.Expect(roundTime.Before(m.LastRoundTime[c.OCR2.Address()])).Should(BeTrue())
		}
	}, NewRoundCheckTimeout, NewRoundCheckPollInterval).Should(Succeed())
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
	Eventually(func(g Gomega) {
		answers := make(map[string]*Answer)
		for _, c := range m.Contracts {
			answer, timestamp, _, err := c.Store.GetLatestRoundData()
			g.Expect(err).ShouldNot(HaveOccurred())
			answers[c.OCR2.Address()] = &Answer{Answer: answer, Timestamp: timestamp, Error: err}
		}
		for ci, a := range answers {
			log.Debug().Str("Contract", ci).Interface("Answer", a).Msg("Answer found")
			answerTime := time.Unix(int64(a.Timestamp), 0)
			if answerTime.After(m.LastRoundTime[ci]) {
				m.LastRoundTime[ci] = answerTime
				roundsFound++
				log.Debug().Int("RoundsFound", roundsFound).Send()
			}
		}
		g.Expect(roundsFound).To(BeNumerically(">=", rounds*len(m.Contracts)))
	}, timeout, NewRoundCheckPollInterval).Should(Succeed())
}
