package common

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"

	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/solclient"

	"github.com/rs/zerolog/log"

	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-testing-framework/client"
	"github.com/smartcontractkit/chainlink-testing-framework/contracts"
	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"golang.org/x/crypto/curve25519"
)

const (
	ChainName = "solana"
)

// ContractNodeInfo contains the indexes of the nodes, bridges, NodeKeyBundles and nodes relevant to an OCR2 Contract
type ContractNodeInfo struct {
	OCR2                    *solclient.OCRv2
	Store                   *solclient.Store
	BootstrapNodeIdx        int
	BootstrapNode           client.Chainlink
	BootstrapNodeKeysBundle NodeKeysBundle
	BootstrapBridgeInfo     BridgeInfo
	NodesIdx                []int
	Nodes                   []client.Chainlink
	NodeKeysBundle          []NodeKeysBundle
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

func CreateSolanaChainAndNode(nodes []client.Chainlink) error {
	for _, n := range nodes {
		_, err := n.CreateSolanaChain(&client.SolanaChainAttributes{ChainID: "localnet"})
		if err != nil {
			return err
		}
		_, err = n.CreateSolanaNode(&client.SolanaNodeAttributes{
			Name:          "solana",
			SolanaChainID: "localnet",
			SolanaURL:     "http://sol:8899",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateNodeKeysBundle(nodes []client.Chainlink) ([]NodeKeysBundle, error) {
	nkb := make([]NodeKeysBundle, 0)
	for _, n := range nodes {
		p2pkeys, err := n.ReadP2PKeys()
		if err != nil {
			return nil, err
		}

		peerID := p2pkeys.Data[0].Attributes.PeerID
		txKey, err := n.CreateTxKey(ChainName)
		if err != nil {
			return nil, err
		}
		ocrKey, err := n.CreateOCR2Key(ChainName)
		if err != nil {
			return nil, err
		}
		nkb = append(nkb, NodeKeysBundle{
			PeerID:  peerID,
			OCR2Key: ocrKey,
			TXKey:   txKey,
		})
	}
	return nkb, nil
}

func createOracleIdentities(nkb []NodeKeysBundle) ([]confighelper.OracleIdentityExtra, error) {
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

func FundOracles(c blockchain.EVMClient, nkb []NodeKeysBundle, amount *big.Float) error {
	for _, nk := range nkb {
		addr := nk.TXKey.Data.Attributes.PublicKey
		if err := c.Fund(addr, amount); err != nil {
			return err
		}
	}
	return nil
}

// OffChainConfigParamsFromNodes creates contracts.OffChainAggregatorV2Config
func OffChainConfigParamsFromNodes(nodes []client.Chainlink, nkb []NodeKeysBundle) (contracts.OffChainAggregatorV2Config, error) {
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
	log.Warn().Int("Nodes", faultyNodes).Msg("Faulty nodes")
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
		MaxDurationQuery:                        0,
		MaxDurationObservation:                  500 * time.Millisecond,
		MaxDurationReport:                       500 * time.Millisecond,
		MaxDurationShouldAcceptFinalizedReport:  500 * time.Millisecond,
		MaxDurationShouldTransmitAcceptedReport: 500 * time.Millisecond,
		F:                                       faultyNodes,
		OnchainConfig:                           []byte{},
	}, nil
}

func CreateBridges(ContractsIdxMapToContractsNodeInfo map[int]*ContractNodeInfo, mock *client.MockserverClient) error {
	for i, nodesInfo := range ContractsIdxMapToContractsNodeInfo {
		// Bootstrap node first
		nodeContractPairID, err := BuildNodeContractPairID(nodesInfo.BootstrapNode, nodesInfo.OCR2.Address())
		if err != nil {
			return err
		}
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        nodeContractPairID,
			URL:         fmt.Sprintf("%s/%s", mock.Config.ClusterURL, nodeContractPairID),
			RequestData: "{}",
		}
		observationSource := client.ObservationSourceSpecBridge(sourceValueBridge)
		err = nodesInfo.BootstrapNode.CreateBridge(&sourceValueBridge)
		if err != nil {
			return err
		}
		juelsBridge := client.BridgeTypeAttributes{
			Name:        nodeContractPairID + "juels",
			URL:         fmt.Sprintf("%s/juels", mock.Config.ClusterURL),
			RequestData: "{}",
		}
		juelsSource := client.ObservationSourceSpecBridge(juelsBridge)
		err = nodesInfo.BootstrapNode.CreateBridge(&juelsBridge)
		if err != nil {
			return err
		}
		ContractsIdxMapToContractsNodeInfo[i].BootstrapBridgeInfo = BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource}

		// Other nodes later
		for _, node := range nodesInfo.Nodes {
			nodeContractPairID, err := BuildNodeContractPairID(node, nodesInfo.OCR2.Address())
			if err != nil {
				return err
			}
			sourceValueBridge := client.BridgeTypeAttributes{
				Name:        nodeContractPairID,
				URL:         fmt.Sprintf("%s/%s", mock.Config.ClusterURL, nodeContractPairID),
				RequestData: "{}",
			}
			observationSource := client.ObservationSourceSpecBridge(sourceValueBridge)
			err = node.CreateBridge(&sourceValueBridge)
			if err != nil {
				return err
			}
			juelsBridge := client.BridgeTypeAttributes{
				Name:        nodeContractPairID + "juels",
				URL:         fmt.Sprintf("%s/juels", mock.Config.ClusterURL),
				RequestData: "{}",
			}
			juelsSource := client.ObservationSourceSpecBridge(juelsBridge)
			err = node.CreateBridge(&juelsBridge)
			if err != nil {
				return err
			}
			ContractsIdxMapToContractsNodeInfo[i].BridgeInfos = append(ContractsIdxMapToContractsNodeInfo[i].BridgeInfos, BridgeInfo{ObservationSource: observationSource, JuelsSource: juelsSource})
		}
	}
	return nil
}

func CreateJobsForContract(contractNodeInfo *ContractNodeInfo) error {
	relayConfig := map[string]string{
		"nodeEndpointHTTP": "http://sol:8899",
		"ocr2ProgramID":    contractNodeInfo.OCR2.ProgramAddress(),
		"transmissionsID":  contractNodeInfo.Store.TransmissionsAddress(),
		"storeProgramID":   contractNodeInfo.Store.ProgramAddress(),
		"chainID":          "localnet",
	}
	bootstrapPeers := []client.P2PData{
		{
			RemoteIP:   contractNodeInfo.BootstrapNode.RemoteIP(),
			RemotePort: "6690",
			PeerID:     contractNodeInfo.BootstrapNodeKeysBundle.PeerID,
		},
	}
	jobSpec := &client.OCR2TaskJobSpec{
		Name:                  fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.NewV4().String()),
		JobType:               "bootstrap",
		ContractID:            contractNodeInfo.OCR2.Address(),
		Relay:                 ChainName,
		RelayConfig:           relayConfig,
		PluginType:            "median",
		P2PV2Bootstrappers:    bootstrapPeers,
		OCRKeyBundleID:        contractNodeInfo.BootstrapNodeKeysBundle.OCR2Key.Data.ID,
		TransmitterID:         contractNodeInfo.BootstrapNodeKeysBundle.TXKey.Data.ID,
		ObservationSource:     contractNodeInfo.BootstrapBridgeInfo.ObservationSource,
		JuelsPerFeeCoinSource: contractNodeInfo.BootstrapBridgeInfo.JuelsSource,
		TrackerPollInterval:   15 * time.Second, // faster config checking
	}
	if _, err := contractNodeInfo.BootstrapNode.CreateJob(jobSpec); err != nil {
		return fmt.Errorf("failed creating job for boostrap node: %w", err)
	}
	for nIdx, n := range contractNodeInfo.Nodes {
		jobSpec := &client.OCR2TaskJobSpec{
			Name:                  fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
			JobType:               "offchainreporting2",
			ContractID:            contractNodeInfo.OCR2.Address(),
			Relay:                 ChainName,
			RelayConfig:           relayConfig,
			PluginType:            "median",
			P2PV2Bootstrappers:    bootstrapPeers,
			OCRKeyBundleID:        contractNodeInfo.NodeKeysBundle[nIdx].OCR2Key.Data.ID,
			TransmitterID:         contractNodeInfo.NodeKeysBundle[nIdx].TXKey.Data.ID,
			ObservationSource:     contractNodeInfo.BridgeInfos[nIdx].ObservationSource,
			JuelsPerFeeCoinSource: contractNodeInfo.BridgeInfos[nIdx].JuelsSource,
			TrackerPollInterval:   15 * time.Second, // faster config checking
		}
		if _, err := n.CreateJob(jobSpec); err != nil {
			return fmt.Errorf("failed creating job for node %s: %w", n.URL(), err)
		}
	}
	return nil
}

func BuildNodeContractPairID(node client.Chainlink, ocr2Addr string) (string, error) {
	csaKeys, err := node.ReadCSAKeys()
	if err != nil {
		return "", err
	}
	shortNodeAddr := csaKeys.Data[0].Attributes.PublicKey[2:12]
	shortOCRAddr := ocr2Addr[2:12]
	return strings.ToLower(fmt.Sprintf("node_%s_contract_%s", shortNodeAddr, shortOCRAddr)), nil
}
