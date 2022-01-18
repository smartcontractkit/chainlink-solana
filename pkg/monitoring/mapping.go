package monitoring

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/pb"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"google.golang.org/protobuf/proto"
)

func MakeConfigSetSimplifiedMapping(
	envelope ConfigEnvelope,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	offchainConfig, err := parseOffchainConfig(envelope.ContractConfig.OffchainConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OffchainConfig blob from the program state: %w", err)
	}
	signers, err := json.Marshal(envelope.ContractConfig.Signers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signers: %w", err)
	}
	transmitters, err := json.Marshal(envelope.ContractConfig.Transmitters)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transmitters: %w", err)
	}
	s, err := json.Marshal(int32ArrToInt64Arr(offchainConfig.S))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schedule: %w", err)
	}
	oracles, err := createConfigSetSimplifiedOracles(offchainConfig.OffchainPublicKeys, offchainConfig.PeerIds, envelope.ContractConfig.Transmitters)
	if err != nil {
		return nil, fmt.Errorf("failed to encode oracle set: %w", err)
	}
	out := map[string]interface{}{
		"config_digest":      base64.StdEncoding.EncodeToString(envelope.ContractConfig.ConfigDigest[:]),
		"block_number":       []byte{},
		"signers":            string(signers),
		"transmitters":       string(transmitters),
		"f":                  int32(envelope.ContractConfig.F),
		"delta_progress":     uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds),
		"delta_resend":       uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds),
		"delta_round":        uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds),
		"delta_grace":        uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds),
		"delta_stage":        uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds),
		"r_max":              int64(offchainConfig.RMax),
		"s":                  string(s),
		"oracles":            string(oracles),
		"feed_state_account": feedConfig.GetContractAddress(),
	}
	return out, nil
}

func MakeTransmissionMapping(
	envelope TransmissionEnvelope,
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	data := []byte{}
	if envelope.LatestAnswer != nil {
		data = envelope.LatestAnswer.Bytes()
	}
	out := map[string]interface{}{
		"block_number": []byte{},
		"answer": map[string]interface{}{
			"config_digest": base64.StdEncoding.EncodeToString(envelope.ConfigDigest[:]),
			"epoch":         int64(envelope.Epoch),
			"round":         int32(envelope.Round),
			"data":          data,
			"timestamp":     envelope.LatestTimestamp.Unix(),
		},
		"solana_chain_config": map[string]interface{}{
			"network_name": solanaConfig.NetworkName,
			"network_id":   solanaConfig.NetworkID,
			"chain_id":     solanaConfig.ChainID,
		},
		"feed_config": feedConfig.ToMapping(),
	}
	return out, nil
}

// Helpers

func uint64ToBeBytes(input uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, input)
	return buf
}

func parseOffchainConfig(buf []byte) (*pb.OffchainConfigProto, error) {
	config := &pb.OffchainConfigProto{}
	err := proto.Unmarshal(buf, config)
	return config, err
}

func int32ArrToInt64Arr(xs []uint32) []int64 {
	out := make([]int64, len(xs))
	for i, x := range xs {
		out[i] = int64(x)
	}
	return out
}

func createConfigSetSimplifiedOracles(offchainPublicKeys [][]byte, peerIDs []string, transmitters []types.Account) ([]byte, error) {
	if len(offchainPublicKeys) != len(peerIDs) && len(transmitters) != len(peerIDs) {
		return nil, fmt.Errorf("length missmatch len(offchainPublicKeys)=%d , len(transmitters)=%d, len(peerIDs)=%d", len(offchainPublicKeys), len(transmitters), len(peerIDs))
	}
	out := make([]interface{}, len(transmitters))
	for i := 0; i < len(transmitters); i++ {
		out[i] = map[string]interface{}{
			"transmitter":         transmitters[i],
			"peer_id":             peerIDs[i],
			"offchain_public_key": offchainPublicKeys[i],
		}
	}
	s, err := json.Marshal(out)
	return s, err
}
