package common

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gopkg.in/guregu/null.v4"

	ctfconfig "github.com/smartcontractkit/chainlink-testing-framework/lib/config"
	ctftestenv "github.com/smartcontractkit/chainlink-testing-framework/lib/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/environment"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/pkg/helm/chainlink"
	mockadapter "github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/pkg/helm/mock-adapter"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/k8s/pkg/helm/sol"

	"github.com/smartcontractkit/chainlink-testing-framework/framework/clclient"
	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"

	chainConfig "github.com/smartcontractkit/chainlink-solana/integration-tests/config"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv"
	testenvsol "github.com/smartcontractkit/chainlink-solana/integration-tests/docker/testenv"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	tc "github.com/smartcontractkit/chainlink-solana/integration-tests/testconfig"
)

type Common struct {
	ChainDetails   *ChainDetails
	TestConfig     *tc.TestConfig
	TestEnvDetails *TestEnvDetails
	ClConfig       map[string]interface{}
	EnvConfig      map[string]interface{}
	Env            *environment.Environment
	DockerEnv      *SolCLClusterTestEnv
	AccountDetails *AccountDetails
}

type TestEnvDetails struct {
	TestDuration      time.Duration
	K8Config          *environment.Config
	NodeContainerEnvs map[string]string
}

type ChainDetails struct {
	ChainName             string
	ChainID               string
	RPCUrls               []string
	RPCURLExternal        string
	WSURLExternal         string
	ProgramAddresses      *chainConfig.ProgramAddresses
	MockserverURLInternal string
	MockServerEndpoint    string
}

type SolCLClusterTestEnv struct {
	Sol     *testenvsol.Solana
	Parrot  *ctftestenv.Parrot
	Clients []*clclient.ChainlinkClient
	NodeSet *ns.Output
}

type AccountDetails struct {
	PrivateKey string
	PublicKey  string
}

// ContractNodeInfo contains the indexes of the nodes, bridges, NodeKeyBundles and nodes relevant to an OCR2 Contract
type ContractNodeInfo struct {
	OCR2                    *solclient.OCRv2
	Store                   *solclient.Store
	BootstrapNodeIdx        int
	BootstrapNode           *clclient.ChainlinkClient
	BootstrapNodeKeysBundle clclient.NodeKeysBundle
	BootstrapBridgeInfo     BridgeInfo
	NodesIdx                []int
	Nodes                   []*clclient.ChainlinkClient
	NodeKeysBundle          []clclient.NodeKeysBundle
	BridgeInfos             []BridgeInfo
}

type BridgeInfo struct {
	ObservationSource string
	JuelsSource       string
}

// Those functions may be common with another chains and should be moved to another lib

type NodeKeysBundle = clclient.NodeKeysBundle

func New(testConfig *tc.TestConfig) *Common {
	var c *Common

	// Setting localnet as the default config
	config := chainConfig.LocalNetConfig()
	// Getting the default localnet private key
	privateKey, err := solana.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
	if err != nil {
		panic("Could not decode private localnet private key")
	}
	privateKeyString := fmt.Sprintf("[%s]", formatBuffer([]byte(privateKey)))
	publicKey := privateKey.PublicKey().String()

	if *testConfig.Common.Network == "devnet" {
		config = chainConfig.DevnetConfig()
		privateKeyString = *testConfig.Common.PrivateKey

		if len(*testConfig.Common.RPCURLs) > 0 {
			config.RPCUrls = *testConfig.Common.RPCURLs
			config.WSUrls = *testConfig.Common.WsURLs
			config.ProgramAddresses = &chainConfig.ProgramAddresses{
				OCR2:             *testConfig.SolanaConfig.OCR2ProgramID,
				AccessController: *testConfig.SolanaConfig.AccessControllerProgramID,
				Store:            *testConfig.SolanaConfig.StoreProgramID,
			}
		}
	}

	c = &Common{
		ChainDetails: &ChainDetails{
			ChainID:          config.ChainID,
			RPCUrls:          config.RPCUrls,
			ChainName:        config.ChainName,
			ProgramAddresses: config.ProgramAddresses,
		},
		TestConfig: testConfig,
		TestEnvDetails: &TestEnvDetails{
			TestDuration: *testConfig.OCR2.TestDurationParsed,
		},
		AccountDetails: &AccountDetails{
			PrivateKey: privateKeyString,
			PublicKey:  publicKey,
		},
		Env: &environment.Environment{},
	}
	// provide getters for TestConfig (pointers to chain details)
	c.TestConfig.GetChainID = func() string { return c.ChainDetails.ChainID }
	c.TestConfig.GetURL = func() []string { return c.ChainDetails.RPCUrls }

	return c
}

func (c *Common) CreateNodeKeysBundle(nodes []*clclient.ChainlinkClient) ([]clclient.NodeKeysBundle, error) {
	nkb := make([]clclient.NodeKeysBundle, 0)
	for _, n := range nodes {
		p2pkeys, err := n.MustReadP2PKeys()
		if err != nil {
			return nil, err
		}

		peerID := p2pkeys.Data[0].Attributes.PeerID
		txKey, _, err := n.CreateTxKey(c.ChainDetails.ChainName, c.ChainDetails.ChainID)
		if err != nil {
			return nil, err
		}
		ocrKey, _, err := n.CreateOCR2Key(c.ChainDetails.ChainName)
		if err != nil {
			return nil, err
		}
		nkb = append(nkb, clclient.NodeKeysBundle{
			PeerID:  peerID,
			OCR2Key: *ocrKey,
			TXKey:   *txKey,
		})
	}
	return nkb, nil
}

func FundOracles(c *solclient.Client, nkb []clclient.NodeKeysBundle, amount *big.Float) error {
	for _, nk := range nkb {
		addr := nk.TXKey.Data.Attributes.PublicKey
		if err := c.Fund(addr, amount); err != nil {
			return err
		}
	}
	return nil
}

func CreateBridges(ContractsIdxMapToContractsNodeInfo map[int]*ContractNodeInfo, mockURL string, _ bool) error {
	for i, nodesInfo := range ContractsIdxMapToContractsNodeInfo {
		nodeContractPairID, err := BuildNodeContractPairID(nodesInfo.BootstrapNode, nodesInfo.OCR2.Address())
		if err != nil {
			return err
		}
		sourceValueBridge := clclient.BridgeTypeAttributes{
			Name:        nodeContractPairID,
			URL:         fmt.Sprintf("%s/%s", mockURL, "five"),
			RequestData: "{}",
		}
		observationSource := clclient.ObservationSourceSpecBridge(&sourceValueBridge)
		err = nodesInfo.BootstrapNode.MustCreateBridge(&sourceValueBridge)
		if err != nil {
			return err
		}
		juelsBridge := clclient.BridgeTypeAttributes{
			Name:        nodeContractPairID + "juels",
			URL:         fmt.Sprintf("%s/%s", mockURL, "five"),
			RequestData: "{}",
		}
		juelsSource := clclient.ObservationSourceSpecBridge(&juelsBridge)
		err = nodesInfo.BootstrapNode.MustCreateBridge(&juelsBridge)
		if err != nil {
			return err
		}
		ContractsIdxMapToContractsNodeInfo[i].BootstrapBridgeInfo = BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource}
		for j := 0; j < len(nodesInfo.Nodes); j++ {
			clClient := nodesInfo.Nodes[j]
			nodeContractPairID, err := BuildNodeContractPairID(clClient, nodesInfo.OCR2.Address())
			if err != nil {
				return err
			}
			sourceValueBridge := clclient.BridgeTypeAttributes{
				Name:        nodeContractPairID,
				URL:         fmt.Sprintf("%s/%s", mockURL, "five"),
				RequestData: "{}",
			}
			observationSource := clclient.ObservationSourceSpecBridge(&sourceValueBridge)
			err = nodesInfo.Nodes[j].MustCreateBridge(&sourceValueBridge)
			if err != nil {
				return err
			}
			juelsBridge := clclient.BridgeTypeAttributes{
				Name:        nodeContractPairID + "juels",
				URL:         fmt.Sprintf("%s/%s", mockURL, "five"),
				RequestData: "{}",
			}
			juelsSource := clclient.ObservationSourceSpecBridge(&juelsBridge)
			err = nodesInfo.Nodes[j].MustCreateBridge(&juelsBridge)
			if err != nil {
				return err
			}
			ContractsIdxMapToContractsNodeInfo[i].BridgeInfos = append(ContractsIdxMapToContractsNodeInfo[i].BridgeInfos, BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource})
		}
	}
	return nil
}

func PluginConfigToTomlFormat(pluginConfig string) devenv.JSONConfig {
	return devenv.JSONConfig{
		"juelsPerFeeCoinSource": fmt.Sprintf("\"\"\"\n%s\n\"\"\"", pluginConfig),
	}
}

func (c *Common) CreateJobsForContract(contractNodeInfo *ContractNodeInfo) error {
	bootstrapNodeInternalIP := contractNodeInfo.BootstrapNode.InternalIP()
	nodeCount := len(contractNodeInfo.Nodes)
	relayConfig := devenv.JSONConfig{
		"nodeEndpointHTTP": c.ChainDetails.RPCUrls,
		"ocr2ProgramID":    contractNodeInfo.OCR2.ProgramAddress(),
		"transmissionsID":  contractNodeInfo.Store.TransmissionsAddress(),
		"storeProgramID":   contractNodeInfo.Store.ProgramAddress(),
		"chainID":          c.ChainDetails.ChainID,
	}
	bootstrapPeers := []clclient.P2PData{
		{
			InternalIP:   bootstrapNodeInternalIP,
			InternalPort: "6690",
			PeerID:       contractNodeInfo.BootstrapNodeKeysBundle.PeerID,
		},
	}
	jobSpec := &devenv.TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.New().String()),
		JobType: "bootstrap",
		OCR2OracleSpec: devenv.OracleSpec{
			ContractID:                        contractNodeInfo.OCR2.Address(),
			Relay:                             c.ChainDetails.ChainName,
			RelayConfig:                       relayConfig,
			P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
			OCRKeyBundleID:                    null.StringFrom(contractNodeInfo.BootstrapNodeKeysBundle.OCR2Key.Data.ID),
			TransmitterID:                     null.StringFrom(contractNodeInfo.BootstrapNodeKeysBundle.TXKey.Data.ID),
			ContractConfigConfirmations:       1,
			ContractConfigTrackerPollInterval: *devenv.NewInterval(15 * time.Second),
		},
	}
	if _, err := contractNodeInfo.BootstrapNode.MustCreateJob(jobSpec); err != nil {
		s, _ := jobSpec.String()
		return fmt.Errorf("failed creating job for boostrap node: %w\n spec:\n%s", err, s)
	}

	for nIdx := 0; nIdx < nodeCount; nIdx++ {
		jobSpec := &devenv.TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.New().String()),
			JobType:           "offchainreporting2",
			ObservationSource: contractNodeInfo.BridgeInfos[nIdx].ObservationSource,
			OCR2OracleSpec: devenv.OracleSpec{
				ContractID:                        contractNodeInfo.OCR2.Address(),
				Relay:                             c.ChainDetails.ChainName,
				RelayConfig:                       relayConfig,
				P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
				OCRKeyBundleID:                    null.StringFrom(contractNodeInfo.NodeKeysBundle[nIdx].OCR2Key.Data.ID),
				TransmitterID:                     null.StringFrom(contractNodeInfo.NodeKeysBundle[nIdx].TXKey.Data.ID),
				ContractConfigConfirmations:       1,
				ContractConfigTrackerPollInterval: *devenv.NewInterval(15 * time.Second),
				PluginType:                        "median",
				PluginConfig:                      PluginConfigToTomlFormat(contractNodeInfo.BridgeInfos[nIdx].JuelsSource),
			},
		}
		n := contractNodeInfo.Nodes[nIdx]
		if _, err := n.MustCreateJob(jobSpec); err != nil {
			return fmt.Errorf("failed creating job for node %s: %w", n.URL(), err)
		}
	}
	return nil
}

func BuildNodeContractPairID(node *clclient.ChainlinkClient, ocr2Addr string) (string, error) {
	csaKeys, resp, err := node.ReadCSAKeys()
	if err != nil {
		return "", err
	}
	if len(csaKeys.Data) <= 0 {
		return "", fmt.Errorf("no csa key data was found on the node %v", resp)
	}
	shortNodeAddr := csaKeys.Data[0].Attributes.PublicKey[2:12]
	shortOCRAddr := ocr2Addr[2:12]
	return strings.ToLower(fmt.Sprintf("node_%s_contract_%s", shortNodeAddr, shortOCRAddr)), nil
}

func (c *Common) Default(t *testing.T, namespacePrefix string) (*Common, error) {
	productName := "data-feedsv2.0"
	nsLabels, err := environment.GetRequiredChainLinkNamespaceLabels(productName, "soak")
	if err != nil {
		return nil, err
	}

	workloadPodLabels, err := environment.GetRequiredChainLinkWorkloadAndPodLabels(productName, "soak")
	if err != nil {
		return nil, err
	}

	c.TestEnvDetails.K8Config = &environment.Config{
		NamespacePrefix: fmt.Sprintf("solana-%s", namespacePrefix),
		TTL:             c.TestEnvDetails.TestDuration,
		Test:            t,
		Labels:          nsLabels,
		WorkloadLabels:  workloadPodLabels,
		PodLabels:       workloadPodLabels,
	}

	if *c.TestConfig.Common.InsideK8s {
		tomlString, err := c.TestConfig.GetNodeConfigTOML()
		if err != nil {
			return nil, err
		}
		var overrideFn = func(_ interface{}, target interface{}) {
			ctfconfig.MustConfigOverrideChainlinkVersion(c.TestConfig.ChainlinkImage, target)
		}
		cd := chainlink.NewWithOverride(0, map[string]any{
			"toml":     tomlString,
			"replicas": *c.TestConfig.OCR2.NodeCount,
			"chainlink": map[string]interface{}{
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "2000m",
						"memory": "4Gi",
					},
					"limits": map[string]interface{}{
						"cpu":    "2000m",
						"memory": "4Gi",
					},
				},
			},
			"db": map[string]any{
				"image": map[string]any{
					"version": "15.5",
				},
				"stateful": c.TestConfig.Common.Stateful,
			},
		}, c.TestConfig.ChainlinkImage, overrideFn)
		c.Env = environment.New(c.TestEnvDetails.K8Config).
			AddHelm(sol.New(nil)).
			AddHelm(mockadapter.New(nil)).
			AddHelm(cd)
	}

	return c, nil
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
