package smoke

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"golang.org/x/crypto/curve25519"
)

const (
	ChainName = "solana"
)

// UploadProgramBinaries uploads programs binary files to solana-validator container
// currently it's the only way to deploy anything to local solana because ephemeral validator in k8s
// can't expose UDP ports required to copy .so chunks when deploying
func UploadProgramBinaries(e *environment.Environment) error {
	connections := e.Charts.Connections("solana-validator")
	cc, err := connections.Load("sol", "0", "sol-val")
	if err != nil {
		return err
	}
	_, _, _, err = e.Charts["solana-validator"].CopyToPod(utils.ContractsDir, fmt.Sprintf("%s/%s:/programs", e.Namespace, cc.PodName), "sol-val")
	if err != nil {
		return err
	}
	return nil
}

type NodeKeysBundle struct {
	OCR2Key *client.OCR2Key
	PeerID  string
	TXKey   *client.TxKey
}

// ocr2 keys are in format ocr2<key_type>_<network>_<key>
func stripKeyPrefix(key string) string {
	chunks := strings.Split(key, "_")
	if len(chunks) == 3 {
		return chunks[2]
	}
	return key
}

func createNodeKeys(nodes []client.Chainlink) ([]NodeKeysBundle, error) {
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
		offChainPubKey, err := hex.DecodeString(stripKeyPrefix(nodeKeys.OCR2Key.Data.Attributes.OffChainPublicKey))
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

func TriggerNewRound(ms *client.MockserverClient, round int, min int, max int) error {
	if round%2 == 0 {
		if err := ms.SetValuePath("/variable", min); err != nil {
			return err
		}
	} else {
		if err := ms.SetValuePath("/variable", max); err != nil {
			return err
		}
	}
	return nil
}

func CreateOCR2Jobs(
	nodes []client.Chainlink,
	nkb []NodeKeysBundle,
	mockserver *client.MockserverClient,
	ocr2 contracts.OCRv2,
	validator contracts.OCRv2DeviationFlaggingValidator) error {
	relayConfig := map[string]string{
		"nodeEndpointHTTP":   "http://sol:8899",
		"nodeEndpointWS":     "ws://sol:8900",
		"stateID":            ocr2.Address(),
		"transmissionsID":    ocr2.TransmissionsAddr(),
		"validatorProgramID": validator.ProgramAddress(),
	}
	bootstrapPeers := []client.P2PData{
		{
			RemoteIP:   nodes[0].RemoteIP(),
			RemotePort: "6690",
			PeerID:     nkb[0].PeerID,
		},
	}
	for nIdx, n := range nodes {
		var IsBootstrapPeer bool
		if nIdx == 0 {
			IsBootstrapPeer = true
		}
		sourceValueBridge := client.BridgeTypeAttributes{
			Name:        "variable",
			URL:         fmt.Sprintf("%s/variable", mockserver.Config.ClusterURL),
			RequestData: "{}",
		}
		observationSource := client.ObservationSourceSpecBridge(sourceValueBridge)
		if err := n.CreateBridge(&sourceValueBridge); err != nil {
			return err
		}

		juelsBridge := client.BridgeTypeAttributes{
			Name:        "juels",
			URL:         fmt.Sprintf("%s/juels", mockserver.Config.ClusterURL),
			RequestData: "{}",
		}
		juelsSource := client.ObservationSourceSpecBridge(juelsBridge)
		if err := n.CreateBridge(&juelsBridge); err != nil {
			return err
		}
		jobSpec := &client.OCR2TaskJobSpec{
			Name:                  fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
			ContractID:            ocr2.ProgramAddress(),
			Relay:                 ChainName,
			RelayConfig:           relayConfig,
			P2PPeerID:             nkb[nIdx].PeerID,
			P2PBootstrapPeers:     bootstrapPeers,
			IsBootstrapPeer:       IsBootstrapPeer,
			OCRKeyBundleID:        nkb[nIdx].OCR2Key.Data.ID,
			TransmitterID:         nkb[nIdx].TXKey.Data.ID,
			ObservationSource:     observationSource,
			JuelsPerFeeCoinSource: juelsSource,
		}
		_, err := n.CreateJob(jobSpec)
		if err != nil {
			return err
		}
	}
	return nil
}

func FundOracles(c client.BlockchainClient, nkb []NodeKeysBundle, amount *big.Float) error {
	for _, nk := range nkb {
		addr := nk.TXKey.Data.Attributes.PublicKey
		if err := c.Fund(addr, amount); err != nil {
			return err
		}
	}
	return nil
}

// DefaultOffChainConfigParamsFromNodes collects OCR2 keys and creates contracts.OffChainAggregatorV2Config
func DefaultOffChainConfigParamsFromNodes(nodes []client.Chainlink) (contracts.OffChainAggregatorV2Config, []NodeKeysBundle, error) {
	nkb, err := createNodeKeys(nodes)
	if err != nil {
		return contracts.OffChainAggregatorV2Config{}, nil, err
	}
	oi, err := createOracleIdentities(nkb[1:])
	if err != nil {
		return contracts.OffChainAggregatorV2Config{}, nil, err
	}
	alphaPPB := uint64(1000000)
	pluginConfig := median.OffchainConfig{
		AlphaReportPPB: alphaPPB,
		AlphaAcceptPPB: alphaPPB,
	}.Encode()
	return contracts.OffChainAggregatorV2Config{
		DeltaProgress:                           2 * time.Second,
		DeltaResend:                             5 * time.Second,
		DeltaRound:                              1 * time.Second,
		DeltaGrace:                              500 * time.Millisecond,
		DeltaStage:                              5 * time.Second,
		RMax:                                    3,
		S:                                       []int{1, 1, 1, 1},
		Oracles:                                 oi,
		ReportingPluginConfig:                   pluginConfig,
		MaxDurationQuery:                        500 * time.Millisecond,
		MaxDurationObservation:                  500 * time.Millisecond,
		MaxDurationReport:                       500 * time.Millisecond,
		MaxDurationShouldAcceptFinalizedReport:  2 * time.Second,
		MaxDurationShouldTransmitAcceptedReport: 2 * time.Second,
		F:                                       1,
		OnchainConfig:                           []byte{},
	}, nkb, nil
}
