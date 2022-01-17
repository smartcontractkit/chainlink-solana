package monitoring

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/mr-tron/base58"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/pb"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"google.golang.org/protobuf/proto"
)

func MakeConfigSetMapping(
	envelope StateEnvelope,
	solanaConfig config.Solana,
	feedConfig Feed,
) (map[string]interface{}, error) {
	offchainConfig, err := parseOffchainConfig(envelope.State.Config.OffchainConfig.Raw[:envelope.State.Config.OffchainConfig.Len])
	if err != nil {
		return nil, fmt.Errorf("failed to parse OffchainConfig blob from the program state: %w", err)
	}
	numericalMedianOffchainConfig, err := parseNumericalMedianOffchainConfig(offchainConfig.ReportingPluginConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ReportingPluginConfig from OffchainConfig: %w", err)
	}
	sharedSecredEncryptions := map[string]interface{}{
		"diffie_hellman_point": []byte{},
		"shared_secret_hash":   []byte{},
		"encryptions":          []byte{},
	}
	if offchainConfig.SharedSecretEncryptions != nil {
		sharedSecredEncryptions = map[string]interface{}{
			"diffie_hellman_point": offchainConfig.SharedSecretEncryptions.DiffieHellmanPoint,
			"shared_secret_hash":   offchainConfig.SharedSecretEncryptions.SharedSecretHash,
			"encryptions":          offchainConfig.SharedSecretEncryptions.Encryptions,
		}
	}
	out := map[string]interface{}{
		"block_number": uint64ToBeBytes(envelope.BlockNumber),
		"contract_config": map[string]interface{}{
			"config_digest": envelope.State.Config.LatestConfigDigest[:],
			"config_count":  int64(envelope.State.Config.ConfigCount),
			"signers":       extractSigners(envelope.State.Oracles),
			"transmitters":  extractTransmitters(envelope.State.Oracles),
			"f":             int32(envelope.State.Config.F),
			"onchain_config": map[string]interface{}{
				"link.chain.ocr2.ocr2_numerical_median_onchain_config": map[string]interface{}{
					"min": envelope.State.Config.MinAnswer.BigInt().Bytes(),
					"max": envelope.State.Config.MaxAnswer.BigInt().Bytes(),
				},
			},
			"offchain_config_version": uint64ToBeBytes(envelope.State.Config.OffchainConfig.Version),
			"offchain_config": map[string]interface{}{
				"link.chain.ocr2.ocr2_offchain_config": map[string]interface{}{
					"delta_progress_nanoseconds": uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds),
					"delta_resend_nanoseconds":   uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds),
					"delta_round_nanoseconds":    uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds),
					"delta_grace_nanoseconds":    uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds),
					"delta_stage_nanoseconds":    uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds),
					"r_max":                      int64(offchainConfig.RMax),
					"s":                          int32ArrToInt64Arr(offchainConfig.S),
					"offchain_public_keys":       offchainConfig.OffchainPublicKeys,
					"peer_ids":                   offchainConfig.PeerIds,
					"reporting_plugin_config": map[string]interface{}{
						"link.chain.ocr2.ocr2_numerical_median_offchain_config": map[string]interface{}{
							"alpha_report_infinite": numericalMedianOffchainConfig.AlphaReportInfinite,
							"alpha_report_ppb":      uint64ToBeBytes(numericalMedianOffchainConfig.AlphaReportPpb),
							"alpha_accept_infinite": numericalMedianOffchainConfig.AlphaAcceptInfinite,
							"alpha_accept_ppb":      uint64ToBeBytes(numericalMedianOffchainConfig.AlphaAcceptPpb),
							"delta_c_nanoseconds":   uint64ToBeBytes(numericalMedianOffchainConfig.DeltaCNanoseconds),
						},
					},
					"max_duration_query_nanoseconds":                           uint64ToBeBytes(offchainConfig.MaxDurationQueryNanoseconds),
					"max_duration_observation_nanoseconds":                     uint64ToBeBytes(offchainConfig.MaxDurationObservationNanoseconds),
					"max_duration_report_nanoseconds":                          uint64ToBeBytes(offchainConfig.MaxDurationReportNanoseconds),
					"max_duration_should_accept_finalized_report_nanoseconds":  uint64ToBeBytes(offchainConfig.MaxDurationShouldAcceptFinalizedReportNanoseconds),
					"max_duration_should_transmit_accepted_report_nanoseconds": uint64ToBeBytes(offchainConfig.MaxDurationShouldTransmitAcceptedReportNanoseconds),
					"shared_secret_encryptions":                                sharedSecredEncryptions,
				},
			},
		},
		"solana_program_state": map[string]interface{}{
			"account_discriminator": envelope.State.AccountDiscriminator[:8],
			"version":               int32(envelope.State.Version),
			"nonce":                 int32(envelope.State.Nonce),
			"config": map[string]interface{}{
				"owner":                       envelope.State.Config.Owner[:],
				"token_mint":                  envelope.State.Config.TokenMint[:],
				"token_vault":                 envelope.State.Config.TokenVault[:],
				"requester_access_controller": envelope.State.Config.RequesterAccessController[:],
				"billing_access_controller":   envelope.State.Config.BillingAccessController[:],
				"min_answer":                  envelope.State.Config.MinAnswer.BigInt().Bytes(),
				"max_answer":                  envelope.State.Config.MaxAnswer.BigInt().Bytes(),
				"f":                           int32(envelope.State.Config.F),
				"round":                       int32(envelope.State.Config.Round),
				"epoch":                       int64(envelope.State.Config.Epoch),
				"latest_aggregator_round_id":  int64(envelope.State.Config.LatestAggregatorRoundID),
				"latest_transmitter":          envelope.State.Config.LatestTransmitter[:],
				"config_count":                int64(envelope.State.Config.ConfigCount),
				"latest_config_digest":        envelope.State.Config.LatestConfigDigest[:],
				"latest_config_block_number":  uint64ToBeBytes(envelope.State.Config.LatestConfigBlockNumber),
				"billing": map[string]interface{}{
					"observation_payment": int64(envelope.State.Config.Billing.ObservationPayment),
				},
				// These fields (validator, flagging_threshold, decimals, description) have been removed from the program's
				// state but they have been kept here to preserve backwards compatibility.
				"validator":          []byte{},
				"flagging_threshold": 0,
				"decimals":           0,
				"description":        []byte{},
			},
			"oracles":          formatOracles(envelope.State.Oracles),
			"leftover_payment": formatLeftovers(envelope.State.LeftoverPayments),
			"transmissions":    envelope.State.Transmissions[:],
		},
		"solana_chain_config": map[string]interface{}{
			"network_name": solanaConfig.NetworkName,
			"network_id":   solanaConfig.NetworkID,
			"chain_id":     solanaConfig.ChainID,
		},
		"feed_config": map[string]interface{}{
			"feed_name":             feedConfig.FeedName,
			"feed_path":             feedConfig.FeedPath,
			"symbol":                feedConfig.Symbol,
			"heartbeat_sec":         int64(feedConfig.HeartbeatSec),
			"contract_type":         feedConfig.ContractType,
			"contract_status":       feedConfig.ContractStatus,
			"contract_address":      feedConfig.ContractAddress.Bytes(),
			"transmissions_account": feedConfig.TransmissionsAccount.Bytes(),
			"state_account":         feedConfig.StateAccount.Bytes(),
		},
	}
	return out, nil
}

func MakeConfigSetSimplifiedMapping(
	envelope StateEnvelope,
	feedConfig Feed,
) (map[string]interface{}, error) {
	offchainConfig, err := parseOffchainConfig(envelope.State.Config.OffchainConfig.Raw[:envelope.State.Config.OffchainConfig.Len])
	if err != nil {
		return nil, fmt.Errorf("failed to parse OffchainConfig blob from the program state: %w", err)
	}
	signers, err := json.Marshal(extractSigners(envelope.State.Oracles))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signers: %w", err)
	}
	transmitters, err := json.Marshal(extractTransmitters(envelope.State.Oracles))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transmitters: %w", err)
	}
	s, err := json.Marshal(int32ArrToInt64Arr(offchainConfig.S))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schedule: %w", err)
	}
	oracles, err := createConfigSetSimplifiedOracles(offchainConfig.OffchainPublicKeys, offchainConfig.PeerIds, envelope.State.Oracles)
	if err != nil {
		return nil, fmt.Errorf("failed to encode oracle set: %w", err)
	}
	out := map[string]interface{}{
		"config_digest":      base64.StdEncoding.EncodeToString(envelope.State.Config.LatestConfigDigest[:]),
		"block_number":       uint64ToBeBytes(envelope.BlockNumber),
		"signers":            string(signers),
		"transmitters":       string(transmitters),
		"f":                  int32(envelope.State.Config.F),
		"delta_progress":     uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds),
		"delta_resend":       uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds),
		"delta_round":        uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds),
		"delta_grace":        uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds),
		"delta_stage":        uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds),
		"r_max":              int64(offchainConfig.RMax),
		"s":                  string(s),
		"oracles":            string(oracles),
		"feed_state_account": base58.Encode(feedConfig.StateAccount[:]),
	}
	return out, nil
}

func MakeTransmissionMapping(
	envelope TransmissionEnvelope,
	solanaConfig config.Solana,
	feedConfig Feed,
) (map[string]interface{}, error) {
	data := []byte{}
	if envelope.Answer.Data != nil {
		data = envelope.Answer.Data.Bytes()
	}
	out := map[string]interface{}{
		"block_number": uint64ToBeBytes(envelope.BlockNumber),
		"answer": map[string]interface{}{
			"data":      data,
			"timestamp": int64(envelope.Answer.Timestamp),
		},
		"solana_chain_config": map[string]interface{}{
			"network_name": solanaConfig.NetworkName,
			"network_id":   solanaConfig.NetworkID,
			"chain_id":     solanaConfig.ChainID,
		},
		"feed_config": map[string]interface{}{
			"feed_name":             feedConfig.FeedName,
			"feed_path":             feedConfig.FeedPath,
			"symbol":                feedConfig.Symbol,
			"heartbeat_sec":         int64(feedConfig.HeartbeatSec),
			"contract_type":         feedConfig.ContractType,
			"contract_status":       feedConfig.ContractStatus,
			"contract_address":      feedConfig.ContractAddress.Bytes(),
			"transmissions_account": feedConfig.TransmissionsAccount.Bytes(),
			"state_account":         feedConfig.StateAccount.Bytes(),
		},
	}
	return out, nil
}

// Helpers

func uint64ToBeBytes(input uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, input)
	return buf
}

func extractSigners(oracles pkgSolana.Oracles) []interface{} {
	out := make([]interface{}, oracles.Len)
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		out[i] = oracles.Raw[i].Signer.Key[:]
	}
	return out
}

func extractTransmitters(oracles pkgSolana.Oracles) []interface{} {
	out := make([]interface{}, oracles.Len)
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		out[i] = oracles.Raw[i].Transmitter.Bytes()
	}
	return out
}

func parseOffchainConfig(buf []byte) (*pb.OffchainConfigProto, error) {
	config := &pb.OffchainConfigProto{}
	err := proto.Unmarshal(buf, config)
	return config, err
}

func parseNumericalMedianOffchainConfig(buf []byte) (*pb.NumericalMedianConfigProto, error) {
	config := &pb.NumericalMedianConfigProto{}
	err := proto.Unmarshal(buf, config)
	return config, err
}

func formatOracles(oracles pkgSolana.Oracles) []interface{} {
	out := make([]interface{}, oracles.Len)
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		out[i] = map[string]interface{}{
			"transmitter": oracles.Raw[i].Transmitter[:],
			"signer": map[string]interface{}{
				"key": oracles.Raw[i].Signer.Key[:],
			},
			"payee":         oracles.Raw[i].Payee[:],
			"from_round_id": int64(oracles.Raw[i].FromRoundID),
			"payment":       uint64ToBeBytes(oracles.Raw[i].Payment),
		}
	}
	return out
}

func formatLeftovers(leftovers pkgSolana.LeftoverPayments) []interface{} {
	out := make([]interface{}, leftovers.Len)
	var i uint64
	for i = 0; i < leftovers.Len; i++ {
		out[i] = map[string]interface{}{
			"payee":  leftovers.Raw[i].Payee[:],
			"amount": uint64ToBeBytes(leftovers.Raw[i].Amount),
		}
	}
	return out
}

func int32ArrToInt64Arr(xs []uint32) []int64 {
	out := make([]int64, len(xs))
	for i, x := range xs {
		out[i] = int64(x)
	}
	return out
}

func createConfigSetSimplifiedOracles(offchainPublicKeys [][]byte, peerIDs []string, oracles pkgSolana.Oracles) ([]byte, error) {
	if len(offchainPublicKeys) != len(peerIDs) && oracles.Len != uint64(len(peerIDs)) {
		return nil, fmt.Errorf("length missmatch len(offchainPublicKeys)=%d , oracles.Len=%d, len(peerIDs)=%d", len(offchainPublicKeys), oracles.Len, len(peerIDs))
	}
	out := make([]interface{}, oracles.Len)
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		out[i] = map[string]interface{}{
			"transmitter":         oracles.Raw[i].Transmitter,
			"peer_id":             peerIDs[i],
			"offchain_public_key": offchainPublicKeys[i],
		}
	}
	s, err := json.Marshal(out)
	return s, err
}
