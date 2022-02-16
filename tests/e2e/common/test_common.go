package common

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/gagliardetto/solana-go"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/solclient"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
)

const (
	ContractsStateFile        = "contracts-chaos-state.json"
	NewRoundCheckTimeout      = 120 * time.Second
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

type OCRv2TestState struct {
	Env              *environment.Environment
	ChainlinkNodes   []client.Chainlink
	ContractDeployer *solclient.ContractDeployer
	LinkToken        contracts.LinkToken
	Store            *solclient.Store
	StoreAuth        string
	BillingAC        *solclient.AccessController
	RequesterAC      *solclient.AccessController
	OCR2             *solclient.OCRv2
	OffChainConfig   contracts.OffChainAggregatorV2Config
	NodeKeysBundle   []NodeKeysBundle
	MockServer       *client.MockserverClient
	Networks         *client.Networks
	RoundsFound      int
	LastRoundTime    time.Time
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

func (m *OCRv2TestState) DeployCluster(nodes int, stateful bool) {
	m.DeployEnv(nodes, stateful)
	m.SetupClients()
	if m.Networks.Default.ContractsDeployed() {
		err := m.LoadContracts()
		Expect(err).ShouldNot(HaveOccurred())
		return
	}
	m.DeployContracts()
	err := m.DumpContracts()
	Expect(err).ShouldNot(HaveOccurred())
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
func (m *OCRv2TestState) UploadProgramBinaries() {
	connections := m.Env.Charts.Connections("solana-validator")
	cc, err := connections.Load("sol", "0", "sol-val")
	Expect(err).ShouldNot(HaveOccurred())
	_, _, _, err = m.Env.Charts["solana-validator"].CopyToPod(utils.ContractsDir, fmt.Sprintf("%s/%s:/programs", m.Env.Namespace, cc.PodName), "sol-val")
	Expect(err).ShouldNot(HaveOccurred())
}

func (m *OCRv2TestState) DeployEnv(nodes int, stateful bool) {
	m.Env, m.err = environment.DeployOrLoadEnvironment(
		solclient.NewChainlinkSolOCRv2(nodes, stateful),
		tools.ChartsRoot,
	)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.Env.ConnectAll()
	Expect(m.err).ShouldNot(HaveOccurred())
	m.UploadProgramBinaries()
}

func (m *OCRv2TestState) SetupClients() {
	networkRegistry := client.NewNetworkRegistry()
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

func (m *OCRv2TestState) DumpContracts() error {
	s := ContractsState{Feed: m.Store.Feed.PrivateKey.String()}
	d, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(ContractsStateFile, d, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (m *OCRv2TestState) LoadContracts() error {
	d, err := os.ReadFile(ContractsStateFile)
	if err != nil {
		return err
	}
	var contractsState *ContractsState
	if err := json.Unmarshal(d, &contractsState); err != nil {
		return err
	}
	feedWallet, err := solana.WalletFromPrivateKeyBase58(contractsState.Feed)
	if err != nil {
		return err
	}
	m.Store = &solclient.Store{
		Client: m.Networks.Default.(*solclient.Client),
		Store:  nil,
		Feed:   feedWallet,
	}
	return nil
}

func (m *OCRv2TestState) DeployContracts() {
	m.OffChainConfig, m.NodeKeysBundle, m.err = DefaultOffChainConfigParamsFromNodes(m.ChainlinkNodes)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.ContractDeployer, m.err = solclient.NewContractDeployer(m.Networks.Default, m.Env)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.LinkToken, m.err = m.ContractDeployer.DeployLinkTokenContract()
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = FundOracles(m.Networks.Default, m.NodeKeysBundle, big.NewFloat(5e4))
	Expect(m.err).ShouldNot(HaveOccurred())
	m.BillingAC, m.err = m.ContractDeployer.DeployOCRv2AccessController()
	Expect(m.err).ShouldNot(HaveOccurred())
	m.RequesterAC, m.err = m.ContractDeployer.DeployOCRv2AccessController()
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.Networks.Default.WaitForEvents()
	Expect(m.err).ShouldNot(HaveOccurred())

	m.Store, m.err = m.ContractDeployer.DeployOCRv2Store(m.BillingAC.Address())
	Expect(m.err).ShouldNot(HaveOccurred())

	m.err = m.Store.CreateFeed("Feed", uint8(18), 10, 1024)
	Expect(m.err).ShouldNot(HaveOccurred())

	m.OCR2, m.err = m.ContractDeployer.DeployOCRv2(m.BillingAC.Address(), m.RequesterAC.Address(), m.LinkToken.Address())
	Expect(m.err).ShouldNot(HaveOccurred())

	m.err = m.OCR2.SetBilling(uint32(1), uint32(1), m.BillingAC.Address())
	Expect(m.err).ShouldNot(HaveOccurred())
	m.StoreAuth, m.err = m.OCR2.AuthorityAddr("store")
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.BillingAC.AddAccess(m.StoreAuth)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.Networks.Default.WaitForEvents()
	Expect(m.err).ShouldNot(HaveOccurred())

	m.err = m.Store.SetWriter(m.StoreAuth)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.Store.SetValidatorConfig(80000)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.Networks.Default.WaitForEvents()
	Expect(m.err).ShouldNot(HaveOccurred())

	m.err = m.OCR2.Configure(m.OffChainConfig)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.err = m.OCR2.DumpState()
	Expect(m.err).ShouldNot(HaveOccurred())
}

func (m *OCRv2TestState) createJobs() {
	relayConfig := map[string]string{
		"nodeEndpointHTTP": "http://sol:8899",
		"ocr2ProgramID":    m.OCR2.ProgramAddress(),
		"transmissionsID":  m.Store.TransmissionsAddress(),
		"storeProgramID":   m.Store.ProgramAddress(),
	}
	bootstrapPeers := []client.P2PData{
		{
			RemoteIP:   m.ChainlinkNodes[0].RemoteIP(),
			RemotePort: "6690",
			PeerID:     m.NodeKeysBundle[0].PeerID,
		},
	}
	for nIdx, n := range m.ChainlinkNodes {
		var IsBootstrapPeer bool
		if nIdx == 0 {
			IsBootstrapPeer = true
		}
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
		jobSpec := &client.OCR2TaskJobSpec{
			Name:                  fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
			ContractID:            m.OCR2.Address(),
			Relay:                 ChainName,
			RelayConfig:           relayConfig,
			P2PPeerID:             m.NodeKeysBundle[nIdx].PeerID,
			P2PBootstrapPeers:     bootstrapPeers,
			IsBootstrapPeer:       IsBootstrapPeer,
			OCRKeyBundleID:        m.NodeKeysBundle[nIdx].OCR2Key.Data.ID,
			TransmitterID:         m.NodeKeysBundle[nIdx].TXKey.Data.ID,
			ObservationSource:     observationSource,
			JuelsPerFeeCoinSource: juelsSource,
			TrackerPollInterval:   10 * time.Second, // faster config checking
		}
		_, err = n.CreateJob(jobSpec)
		Expect(err).ShouldNot(HaveOccurred())
	}
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
	m.SetAllAdapterResponsesToTheSameValue(5)
	m.err = m.MockServer.SetValuePath("/juels", 1)
	Expect(m.err).ShouldNot(HaveOccurred())
	m.createJobs()
}

func (m *OCRv2TestState) ImitateSource(changeInterval time.Duration, min int, max int) {
	go func() {
		for {
			m.SetAllAdapterResponsesToTheSameValue(min)
			time.Sleep(changeInterval)
			m.SetAllAdapterResponsesToTheSameValue(max)
			time.Sleep(changeInterval)
		}
	}()
}

func (m *OCRv2TestState) ValidateNoRoundsAfter(chaosStartTime time.Time) {
	m.RoundsFound = 0
	m.LastRoundTime = chaosStartTime
	Consistently(func(g Gomega) {
		_, timestamp, _, err := m.Store.GetLatestRoundData()
		g.Expect(err).ShouldNot(HaveOccurred())
		roundTime := time.Unix(int64(timestamp), 0)
		g.Expect(roundTime.Before(m.LastRoundTime)).Should(BeTrue())
	}, NewRoundCheckTimeout, NewRoundCheckPollInterval).Should(Succeed())
}

func (m *OCRv2TestState) ValidateRoundsAfter(chaosStartTime time.Time, rounds int) {
	m.RoundsFound = 0
	m.LastRoundTime = chaosStartTime
	Eventually(func(g Gomega) {
		answer, timestamp, _, err := m.Store.GetLatestRoundData()
		g.Expect(err).ShouldNot(HaveOccurred())
		roundTime := time.Unix(int64(timestamp), 0)
		g.Expect(roundTime.After(m.LastRoundTime)).Should(BeTrue())
		m.RoundsFound++
		m.LastRoundTime = roundTime
		log.Debug().
			Int("Rounds", m.RoundsFound).
			Interface("Answer", answer).
			Time("Time", roundTime).
			Msg("OCR Round")
		g.Expect(m.RoundsFound).Should(Equal(rounds))
	}, NewRoundCheckTimeout, NewRoundCheckPollInterval).Should(Succeed())
}
