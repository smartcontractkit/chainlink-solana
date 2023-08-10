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

	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/curve25519"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-env/environment"
	"github.com/smartcontractkit/chainlink-env/pkg/alias"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/chainlink"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/mock-adapter"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/sol"

	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/integration-tests/contracts"
	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/store/models"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
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
	ChainName string
	ChainId   string
	NodeCount int
	TTL       time.Duration
	ClConfig  map[string]interface{}
	EnvConfig map[string]interface{}
	K8Config  *environment.Config
	Env       *environment.Environment
	SolanaUrl string
}

// ContractNodeInfo contains the indexes of the nodes, bridges, NodeKeyBundles and nodes relevant to an OCR2 Contract
type ContractNodeInfo struct {
	OCR2                    *solclient.OCRv2
	Store                   *solclient.Store
	BootstrapNodeIdx        int
	BootstrapNode           *client.ChainlinkK8sClient
	BootstrapNodeKeysBundle client.NodeKeysBundle
	BootstrapBridgeInfo     BridgeInfo
	NodesIdx                []int
	Nodes                   []*client.ChainlinkK8sClient
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

func New(env string) *Common {
	var err error
	var c *Common
	if env == "devnet" {
		c = &Common{
			ChainName: ChainName,
			ChainId:   DevnetChainID,
			SolanaUrl: SolanaDevnetURL,
		}
	} else {
		c = &Common{
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
func OffChainConfigParamsFromNodes(nodes []*client.ChainlinkK8sClient, nkb []client.NodeKeysBundle) (contracts.OffChainAggregatorV2Config, error) {
	oi, err := createOracleIdentities(nkb)
	if err != nil {
		return contracts.OffChainAggregatorV2Config{}, err
	}
	s := make([]int, 0)
	for range nodes {
		s = append(s, 1)
	}
	faultyNodes := 0
	if len(nodes) > 1 {
		faultyNodes = len(nodes)/3 - 1
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

func CreateBridges(ContractsIdxMapToContractsNodeInfo map[int]*ContractNodeInfo, mockUrl string) error {
	for i, nodesInfo := range ContractsIdxMapToContractsNodeInfo {
		// Bootstrap node first
		nodeContractPairID, err := BuildNodeContractPairID(nodesInfo.BootstrapNode.ChainlinkClient, nodesInfo.OCR2.Address())
		if err != nil {
			return err
		}
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        nodeContractPairID,
			URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
			RequestData: "{}",
		}
		observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
		err = nodesInfo.BootstrapNode.MustCreateBridge(&sourceValueBridge)
		if err != nil {
			return err
		}
		juelsBridge := client.BridgeTypeAttributes{
			Name:        nodeContractPairID + "juels",
			URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
			RequestData: "{}",
		}
		juelsSource := client.ObservationSourceSpecBridge(&juelsBridge)
		err = nodesInfo.BootstrapNode.MustCreateBridge(&juelsBridge)
		if err != nil {
			return err
		}
		ContractsIdxMapToContractsNodeInfo[i].BootstrapBridgeInfo = BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource}
		// Other nodes later
		for _, node := range nodesInfo.Nodes {
			nodeContractPairID, err := BuildNodeContractPairID(node.ChainlinkClient, nodesInfo.OCR2.Address())
			if err != nil {
				return err
			}
			sourceValueBridge := client.BridgeTypeAttributes{
				Name:        nodeContractPairID,
				URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
				RequestData: "{}",
			}
			observationSource := client.ObservationSourceSpecBridge(&sourceValueBridge)
			err = node.MustCreateBridge(&sourceValueBridge)
			if err != nil {
				return err
			}
			juelsBridge := client.BridgeTypeAttributes{
				Name:        nodeContractPairID + "juels",
				URL:         fmt.Sprintf("%s/%s", mockUrl, "five"),
				RequestData: "{}",
			}
			juelsSource := client.ObservationSourceSpecBridge(&juelsBridge)
			err = node.MustCreateBridge(&juelsBridge)
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
	relayConfig := job.JSONConfig{
		"nodeEndpointHTTP": fmt.Sprintf("\"%s\"", SolanaLocalNetURL),
		"ocr2ProgramID":    fmt.Sprintf("\"%s\"", contractNodeInfo.OCR2.ProgramAddress()),
		"transmissionsID":  fmt.Sprintf("\"%s\"", contractNodeInfo.Store.TransmissionsAddress()),
		"storeProgramID":   fmt.Sprintf("\"%s\"", contractNodeInfo.Store.ProgramAddress()),
		"chainID":          fmt.Sprintf("\"%s\"", LocalnetChainID),
	}
	bootstrapPeers := []client.P2PData{
		{
			InternalIP:   contractNodeInfo.BootstrapNode.InternalIP(),
			InternalPort: "6690",
			PeerID:       contractNodeInfo.BootstrapNodeKeysBundle.PeerID,
		},
	}
	jobSpec := &client.OCR2TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.NewV4().String()),
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
	if _, err := contractNodeInfo.BootstrapNode.MustCreateJob(jobSpec); err != nil {
		s, _ := jobSpec.String()
		return fmt.Errorf("failed creating job for boostrap node: %w\n spec:\n%s", err, s)
	}
	for nIdx, n := range contractNodeInfo.Nodes {
		jobSpec := &client.OCR2TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
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
		if _, err := n.MustCreateJob(jobSpec); err != nil {
			return fmt.Errorf("failed creating job for node %s: %w", n.URL(), err)
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

func (c *Common) Default(t *testing.T, namespacePrefix string) *Common {
	c.K8Config = &environment.Config{
		NamespacePrefix: fmt.Sprintf("solana-%s", namespacePrefix),
		TTL:             c.TTL,
		Test:            t,
	}
	baseTOML := fmt.Sprintf(`[[Solana]]
Enabled = true
ChainID = '%s'
[[Solana.Nodes]]
Name = 'primary' 
URL = '%s'

[OCR2]
Enabled = true

[P2P]
[P2P.V2]
Enabled = true
DeltaDial = '5s'
DeltaReconcile = '5s'
ListenAddresses = ['0.0.0.0:6690']
`, c.ChainId, c.SolanaUrl)
	c.Env = environment.New(c.K8Config).
		AddHelm(sol.New(nil)).
		AddHelm(mock_adapter.New(nil)).
		AddHelm(chainlink.New(0, map[string]interface{}{
			"toml":     baseTOML,
			"replicas": c.NodeCount,
		}))

	return c
}
