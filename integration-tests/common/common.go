package common

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/curve25519"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	ctf_test_env "github.com/smartcontractkit/chainlink-testing-framework/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/k8s/environment"
	"github.com/smartcontractkit/chainlink-testing-framework/k8s/pkg/alias"
	"github.com/smartcontractkit/chainlink-testing-framework/k8s/pkg/helm/chainlink"
	mock_adapter "github.com/smartcontractkit/chainlink-testing-framework/k8s/pkg/helm/mock-adapter"
	"github.com/smartcontractkit/chainlink-testing-framework/k8s/pkg/helm/sol"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/ptr"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/integration-tests/contracts"
	"github.com/smartcontractkit/chainlink/integration-tests/docker/test_env"
	"github.com/smartcontractkit/chainlink/integration-tests/types/config/node"
	cl "github.com/smartcontractkit/chainlink/v2/core/services/chainlink"
	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/store/models"

	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"

	test_env_sol "github.com/smartcontractkit/chainlink-solana/integration-tests/docker/test_env"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	solcfg "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const (
	ChainName         = "solana"
	LocalnetChainID   = "localnet"
	DevnetChainID     = "devnet"
	DefaultNodeCount  = 5
	DefaultTTL        = "3h"
	SolanaLocalNetURL = "http://sol:8899"
	SolanaDevnetURL   = "https://api.devnet.solana.com"
)

type Common struct {
	IsK8s     bool
	ChainName string
	ChainId   string
	NodeCount int
	NodeOpts  []test_env.ClNodeOption
	TTL       time.Duration
	ClConfig  map[string]interface{}
	EnvConfig map[string]interface{}
	K8Config  *environment.Config
	Env       *environment.Environment
	DockerEnv *SolCLClusterTestEnv
	SolanaUrl string
}

type SolCLClusterTestEnv struct {
	*test_env.CLClusterTestEnv
	Sol       *test_env_sol.Solana
	Killgrave *ctf_test_env.Killgrave
}

// ContractNodeInfo contains the indexes of the nodes, bridges, NodeKeyBundles and nodes relevant to an OCR2 Contract
type ContractNodeInfo struct {
	OCR2                    *solclient.OCRv2
	Store                   *solclient.Store
	BootstrapNodeIdx        int
	BootstrapNode           *client.ChainlinkClient
	BootstrapNodeK8s        *client.ChainlinkK8sClient
	BootstrapNodeKeysBundle client.NodeKeysBundle
	BootstrapBridgeInfo     BridgeInfo
	NodesIdx                []int
	Nodes                   []*client.ChainlinkClient
	NodesK8s                []*client.ChainlinkK8sClient
	NodeKeysBundle          []client.NodeKeysBundle
	BridgeInfos             []BridgeInfo
}

type BridgeInfo struct {
	ObservationSource string
	JuelsSource       string
}

// Those functions may be common with another chains and should be moved to another lib

type NodeKeysBundle struct {
	OCR2Key *client.OCR2Key
	PeerID  string
	TXKey   *client.TxKey
}

// OCR2 keys are in format OCR2<key_type>_<network>_<key>
func stripKeyPrefix(key string) string {
	chunks := strings.Split(key, "_")
	if len(chunks) == 3 {
		return chunks[2]
	}
	return key
}

func New(env string, isK8s bool) *Common {
	var err error
	var c *Common
	if env == "devnet" {
		c = &Common{
			IsK8s:     isK8s,
			ChainName: ChainName,
			ChainId:   DevnetChainID,
			SolanaUrl: SolanaDevnetURL,
		}
	} else {
		c = &Common{
			IsK8s:     isK8s,
			ChainName: ChainName,
			ChainId:   LocalnetChainID,
			SolanaUrl: SolanaLocalNetURL,
		}
	}
	// Checking if count of OCR nodes is defined in ENV
	nodeCountSet, nodeCountDefined := os.LookupEnv("NODE_COUNT")
	if nodeCountDefined && nodeCountSet != "" {
		c.NodeCount, err = strconv.Atoi(nodeCountSet)
		if err != nil {
			panic(fmt.Sprintf("Please define a proper node count for the test: %v", err))
		}
	} else {
		c.NodeCount = DefaultNodeCount
	}

	// Checking if TTL env var is set in ENV
	ttlValue, ttlDefined := os.LookupEnv("TTL")
	if ttlDefined && ttlValue != "" {
		duration, err := time.ParseDuration(ttlValue)
		if err != nil {
			panic(fmt.Sprintf("Please define a proper duration for the test: %v", err))
		}
		c.TTL, err = time.ParseDuration(*alias.ShortDur(duration))
		if err != nil {
			panic(fmt.Sprintf("Please define a proper duration for the test: %v", err))
		}
	} else {
		duration, err := time.ParseDuration(DefaultTTL)
		if err != nil {
			panic(fmt.Sprintf("Please define a proper duration for the test: %v", err))
		}
		c.TTL, err = time.ParseDuration(*alias.ShortDur(duration))
		if err != nil {
			panic(fmt.Sprintf("Please define a proper duration for the test: %v", err))
		}
	}

	return c
}

func (c *Common) CreateSolanaChainAndNode(nodes []*client.ChainlinkClient) error {
	for _, n := range nodes {
		_, _, err := n.CreateSolanaChain(&client.SolanaChainAttributes{ChainID: c.ChainId})
		if err != nil {
			return err
		}
		_, _, err = n.CreateSolanaNode(&client.SolanaNodeAttributes{
			Name:          ChainName,
			SolanaChainID: c.ChainId,
			SolanaURL:     c.SolanaUrl,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Common) CreateNodeKeysBundle(nodes []*client.ChainlinkClient) ([]client.NodeKeysBundle, error) {
	nkb := make([]client.NodeKeysBundle, 0)
	for _, n := range nodes {
		p2pkeys, err := n.MustReadP2PKeys()
		if err != nil {
			return nil, err
		}

		peerID := p2pkeys.Data[0].Attributes.PeerID
		txKey, _, err := n.CreateTxKey(ChainName, c.ChainId)
		if err != nil {
			return nil, err
		}
		ocrKey, _, err := n.CreateOCR2Key(ChainName)
		if err != nil {
			return nil, err
		}
		nkb = append(nkb, client.NodeKeysBundle{
			PeerID:  peerID,
			OCR2Key: *ocrKey,
			TXKey:   *txKey,
		})
	}
	return nkb, nil
}

func createOracleIdentities(nkb []client.NodeKeysBundle) ([]confighelper.OracleIdentityExtra, error) {
	oracleIdentities := make([]confighelper.OracleIdentityExtra, 0)
	for _, nodeKeys := range nkb {
		offChainPubKeyTemp, err := hex.DecodeString(stripKeyPrefix(nodeKeys.OCR2Key.Data.Attributes.OffChainPublicKey))
		if err != nil {
			return nil, err
		}
		onChainPubKey, err := hex.DecodeString(stripKeyPrefix(nodeKeys.OCR2Key.Data.Attributes.OnChainPublicKey))
		if err != nil {
			return nil, err
		}
		cfgPubKeyTemp, err := hex.DecodeString(stripKeyPrefix(nodeKeys.OCR2Key.Data.Attributes.ConfigPublicKey))
		if err != nil {
			return nil, err
		}
		cfgPubKeyBytes := [curve25519.PointSize]byte{}
		copy(cfgPubKeyBytes[:], cfgPubKeyTemp)
		offChainPubKey := [curve25519.PointSize]byte{}
		copy(offChainPubKey[:], offChainPubKeyTemp)
		oracleIdentities = append(oracleIdentities, confighelper.OracleIdentityExtra{
			OracleIdentity: confighelper.OracleIdentity{
				OffchainPublicKey: offChainPubKey,
				OnchainPublicKey:  onChainPubKey,
				PeerID:            nodeKeys.PeerID,
				TransmitAccount:   types.Account(nodeKeys.TXKey.Data.Attributes.PublicKey),
			},
			ConfigEncryptionPublicKey: cfgPubKeyBytes,
		})
	}
	// program sorts oracles (need to pre-sort to allow correct onchainConfig generation)
	sort.Slice(oracleIdentities, func(i, j int) bool {
		return bytes.Compare(oracleIdentities[i].OracleIdentity.OnchainPublicKey, oracleIdentities[j].OracleIdentity.OnchainPublicKey) < 0
	})
	return oracleIdentities, nil
}

func FundOracles(c *solclient.Client, nkb []client.NodeKeysBundle, amount *big.Float) error {
	for _, nk := range nkb {
		addr := nk.TXKey.Data.Attributes.PublicKey
		if err := c.Fund(addr, amount); err != nil {
			return err
		}
	}
	return nil
}

// OffChainConfigParamsFromNodes creates contracts.OffChainAggregatorV2Config
func OffChainConfigParamsFromNodes(nodeCount int, nkb []client.NodeKeysBundle) (contracts.OffChainAggregatorV2Config, error) {
	oi, err := createOracleIdentities(nkb)
	if err != nil {
		return contracts.OffChainAggregatorV2Config{}, err
	}
	s := make([]int, 0)
	for i := 0; i < nodeCount; i++ {
		s = append(s, 1)
	}
	faultyNodes := 0
	if nodeCount > 1 {
		faultyNodes = nodeCount/3 - 1
	}
	if faultyNodes == 0 {
		faultyNodes = 1
	}
	log.Debug().Int("Nodes", faultyNodes).Msg("Faulty nodes")
	return contracts.OffChainAggregatorV2Config{
		DeltaProgress: 2 * time.Second,
		DeltaResend:   5 * time.Second,
		DeltaRound:    1 * time.Second,
		DeltaGrace:    500 * time.Millisecond,
		DeltaStage:    10 * time.Second,
		RMax:          3,
		S:             s,
		Oracles:       oi,
		ReportingPluginConfig: median.OffchainConfig{
			AlphaReportPPB: uint64(0),
			AlphaAcceptPPB: uint64(0),
		}.Encode(),
		MaxDurationQuery:                        20 * time.Millisecond,
		MaxDurationObservation:                  500 * time.Millisecond,
		MaxDurationReport:                       500 * time.Millisecond,
		MaxDurationShouldAcceptFinalizedReport:  500 * time.Millisecond,
		MaxDurationShouldTransmitAcceptedReport: 500 * time.Millisecond,
		F:                                       faultyNodes,
		OnchainConfig:                           []byte{},
	}, nil
}

func CreateBridges(ContractsIdxMapToContractsNodeInfo map[int]*ContractNodeInfo, mockUrl string, isK8s bool) error {
	for i, nodesInfo := range ContractsIdxMapToContractsNodeInfo {
		// Bootstrap node first
		var err error
		var nodeContractPairID string
		if isK8s {
			nodeContractPairID, err = BuildNodeContractPairID(nodesInfo.BootstrapNodeK8s.ChainlinkClient, nodesInfo.OCR2.Address())
		} else {
			nodeContractPairID, err = BuildNodeContractPairID(nodesInfo.BootstrapNode, nodesInfo.OCR2.Address())
		}
		if err != nil {
			return err
		}
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        nodeContractPairID,
			URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
			RequestData: "{}",
		}
		observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
		if isK8s {
			err = nodesInfo.BootstrapNodeK8s.MustCreateBridge(&sourceValueBridge)
		} else {
			err = nodesInfo.BootstrapNode.MustCreateBridge(&sourceValueBridge)
		}
		if err != nil {
			return err
		}
		juelsBridge := client.BridgeTypeAttributes{
			Name:        nodeContractPairID + "juels",
			URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
			RequestData: "{}",
		}
		juelsSource := client.ObservationSourceSpecBridge(&juelsBridge)
		if isK8s {
			err = nodesInfo.BootstrapNodeK8s.MustCreateBridge(&juelsBridge)
		} else {
			err = nodesInfo.BootstrapNode.MustCreateBridge(&juelsBridge)
		}
		if err != nil {
			return err
		}
		ContractsIdxMapToContractsNodeInfo[i].BootstrapBridgeInfo = BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource}
		// Other nodes later
		var nodeCount int
		if isK8s {
			nodeCount = len(nodesInfo.NodesK8s)
		} else {
			nodeCount = len(nodesInfo.Nodes)
		}
		for j := 0; j < nodeCount; j++ {
			var clClient *client.ChainlinkClient
			if isK8s {
				clClient = nodesInfo.NodesK8s[j].ChainlinkClient
			} else {
				clClient = nodesInfo.Nodes[j]
			}
			nodeContractPairID, err := BuildNodeContractPairID(clClient, nodesInfo.OCR2.Address())
			if err != nil {
				return err
			}
			sourceValueBridge := client.BridgeTypeAttributes{
				Name:        nodeContractPairID,
				URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
				RequestData: "{}",
			}
			observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
			if isK8s {
				err = nodesInfo.NodesK8s[j].MustCreateBridge(&sourceValueBridge)
			} else {
				err = nodesInfo.Nodes[j].MustCreateBridge(&sourceValueBridge)
			}
			if err != nil {
				return err
			}
			juelsBridge := client.BridgeTypeAttributes{
				Name:        nodeContractPairID + "juels",
				URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
				RequestData: "{}",
			}
			juelsSource := client.ObservationSourceSpecBridge(&juelsBridge)
			if isK8s {
				err = nodesInfo.NodesK8s[j].MustCreateBridge(&juelsBridge)
			} else {
				err = nodesInfo.Nodes[j].MustCreateBridge(&juelsBridge)
			}
			if err != nil {
				return err
			}
			ContractsIdxMapToContractsNodeInfo[i].BridgeInfos = append(ContractsIdxMapToContractsNodeInfo[i].BridgeInfos, BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource})
		}
	}
	return nil
}

func PluginConfigToTomlFormat(pluginConfig string) job.JSONConfig {
	return job.JSONConfig{
		"juelsPerFeeCoinSource": fmt.Sprintf("\"\"\"\n%s\n\"\"\"", pluginConfig),
	}
}

func (c *Common) CreateJobsForContract(contractNodeInfo *ContractNodeInfo) error {
	var bootstrapNodeInternalIP string
	var nodeCount int
	if c.IsK8s {
		nodeCount = len(contractNodeInfo.NodesK8s)
		bootstrapNodeInternalIP = contractNodeInfo.BootstrapNodeK8s.InternalIP()
	} else {
		nodeCount = len(contractNodeInfo.Nodes)
		bootstrapNodeInternalIP = contractNodeInfo.BootstrapNode.InternalIP()
	}
	relayConfig := job.JSONConfig{
		"nodeEndpointHTTP": SolanaLocalNetURL,
		"ocr2ProgramID":    contractNodeInfo.OCR2.ProgramAddress(),
		"transmissionsID":  contractNodeInfo.Store.TransmissionsAddress(),
		"storeProgramID":   contractNodeInfo.Store.ProgramAddress(),
		"chainID":          LocalnetChainID,
	}
	bootstrapPeers := []client.P2PData{
		{
			InternalIP:   bootstrapNodeInternalIP,
			InternalPort: "6690",
			PeerID:       contractNodeInfo.BootstrapNodeKeysBundle.PeerID,
		},
	}
	jobSpec := &client.OCR2TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.New().String()),
		JobType: "bootstrap",
		OCR2OracleSpec: job.OCR2OracleSpec{
			ContractID:                        contractNodeInfo.OCR2.Address(),
			Relay:                             ChainName,
			RelayConfig:                       relayConfig,
			P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
			OCRKeyBundleID:                    null.StringFrom(contractNodeInfo.BootstrapNodeKeysBundle.OCR2Key.Data.ID),
			TransmitterID:                     null.StringFrom(contractNodeInfo.BootstrapNodeKeysBundle.TXKey.Data.ID),
			ContractConfigConfirmations:       1,
			ContractConfigTrackerPollInterval: models.Interval(15 * time.Second),
		},
	}
	if c.IsK8s {
		if _, err := contractNodeInfo.BootstrapNodeK8s.MustCreateJob(jobSpec); err != nil {
			s, _ := jobSpec.String()
			return fmt.Errorf("failed creating job for boostrap node: %w\n spec:\n%s", err, s)
		}
	} else {
		if _, err := contractNodeInfo.BootstrapNode.MustCreateJob(jobSpec); err != nil {
			s, _ := jobSpec.String()
			return fmt.Errorf("failed creating job for boostrap node: %w\n spec:\n%s", err, s)
		}
	}

	for nIdx := 0; nIdx < nodeCount; nIdx++ {
		jobSpec := &client.OCR2TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.New().String()),
			JobType:           "offchainreporting2",
			ObservationSource: contractNodeInfo.BridgeInfos[nIdx].ObservationSource,
			OCR2OracleSpec: job.OCR2OracleSpec{
				ContractID:                        contractNodeInfo.OCR2.Address(),
				Relay:                             ChainName,
				RelayConfig:                       relayConfig,
				P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
				OCRKeyBundleID:                    null.StringFrom(contractNodeInfo.NodeKeysBundle[nIdx].OCR2Key.Data.ID),
				TransmitterID:                     null.StringFrom(contractNodeInfo.NodeKeysBundle[nIdx].TXKey.Data.ID),
				ContractConfigConfirmations:       1,
				ContractConfigTrackerPollInterval: models.Interval(15 * time.Second),
				PluginType:                        "median",
				PluginConfig:                      PluginConfigToTomlFormat(contractNodeInfo.BridgeInfos[nIdx].JuelsSource),
			},
		}
		if c.IsK8s {
			n := contractNodeInfo.NodesK8s[nIdx]
			if _, err := n.MustCreateJob(jobSpec); err != nil {
				return fmt.Errorf("failed creating job for node %s: %w", n.URL(), err)
			}
		} else {
			n := contractNodeInfo.Nodes[nIdx]
			if _, err := n.MustCreateJob(jobSpec); err != nil {
				return fmt.Errorf("failed creating job for node %s: %w", n.URL(), err)
			}
		}
	}
	return nil
}

func BuildNodeContractPairID(node *client.ChainlinkClient, ocr2Addr string) (string, error) {
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

func (c *Common) DefaultNodeConfig() *cl.Config {
	solConfig := solcfg.TOMLConfig{
		Enabled: ptr.Ptr(true),
		ChainID: ptr.Ptr(c.ChainId),
		Nodes: []*solcfg.Node{
			{
				Name: ptr.Ptr("primary"),
				URL:  config.MustParseURL(c.SolanaUrl),
			},
		},
	}
	baseConfig := node.NewBaseConfig()
	baseConfig.Solana = solcfg.TOMLConfigs{
		&solConfig,
	}
	baseConfig.OCR2.Enabled = ptr.Ptr(true)
	baseConfig.P2P.V2.Enabled = ptr.Ptr(true)
	fiveSecondDuration := commonconfig.MustNewDuration(5 * time.Second)

	baseConfig.P2P.V2.DeltaDial = fiveSecondDuration
	baseConfig.P2P.V2.DeltaReconcile = fiveSecondDuration
	baseConfig.P2P.V2.ListenAddresses = &[]string{"0.0.0.0:6690"}

	return baseConfig
}

func (c *Common) Default(t *testing.T, namespacePrefix string) (*Common, error) {
	c.K8Config = &environment.Config{
		NamespacePrefix: fmt.Sprintf("solana-%s", namespacePrefix),
		TTL:             c.TTL,
		Test:            t,
	}

	if c.IsK8s {
		toml := c.DefaultNodeConfig()
		tomlString, err := toml.TOMLString()
		if err != nil {
			return nil, err
		}
		c.Env = environment.New(c.K8Config).
			AddHelm(sol.New(nil)).
			AddHelm(mock_adapter.New(nil)).
			AddHelm(chainlink.New(0, map[string]interface{}{
				"toml":     tomlString,
				"replicas": c.NodeCount,
			}))
	}

	return c, nil
}
