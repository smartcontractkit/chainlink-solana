package monitoring

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/pb"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"google.golang.org/protobuf/proto"
)

func MakeConfigSetMapping(
	envelope StateEnvelope,
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	state := envelope.State
	offchainConfig, err := parseOffchainConfig(state.Config.OffchainConfig.Raw[:state.Config.OffchainConfig.Len])
	if err != nil {
		return nil, fmt.Errorf("failed to parse OffchainConfig blob from the program state: %w", err)
	}
	numericalMedianOffchainConfig, err := parseNumericalMedianOffchainConfig(offchainConfig.ReportingPluginConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ReportingPluginConfig from OffchainConfig: %w", err)
	}
	out := map[string]interface{}{
		"block_number": uint64ToBeBytes(envelope.BlockNumber),
		"contract_config": map[string]interface{}{
			"config_digest": state.Config.LatestConfigDigest[:],
			"config_count":  int64(state.Config.ConfigCount),
			"signers":       extractSigners(state.Oracles),
			"transmitters":  extractTransmitters(state.Oracles),
			"f":             int32(state.Config.F),
			"onchain_config": map[string]interface{}{
				"link.chain.ocr2.ocr2_numerical_median_onchain_config": map[string]interface{}{
					"min": state.Config.MinAnswer.BigInt().Bytes(),
					"max": state.Config.MaxAnswer.BigInt().Bytes(),
				},
			},
			"offchain_config_version": uint64ToBeBytes(state.Config.OffchainConfig.Version),
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
					"shared_secret_encryptions": map[string]interface{}{
						"diffie_hellman_point": offchainConfig.SharedSecretEncryptions.DiffieHellmanPoint,
						"shared_secret_hash":   offchainConfig.SharedSecretEncryptions.SharedSecretHash,
						"encryptions":          offchainConfig.SharedSecretEncryptions.Encryptions,
					},
				},
			},
		},
		"solana_program_state": map[string]interface{}{
			"account_discriminator": state.AccountDiscriminator[:8],
			"version":               int32(state.Version),
			"nonce":                 int32(state.Nonce),
			"config": map[string]interface{}{
				"owner":                       state.Config.Owner[:],
				"token_mint":                  state.Config.TokenMint[:],
				"token_vault":                 state.Config.TokenVault[:],
				"requester_access_controller": state.Config.RequesterAccessController[:],
				"billing_access_controller":   state.Config.BillingAccessController[:],
				"min_answer":                  state.Config.MinAnswer.BigInt().Bytes(),
				"max_answer":                  state.Config.MaxAnswer.BigInt().Bytes(),
				"description":                 state.Config.Description[:],
				"decimals":                    int32(state.Config.Decimals),
				"f":                           int32(state.Config.F),
				"round":                       int32(state.Config.Round),
				"epoch":                       int64(state.Config.Epoch),
				"latest_aggregator_round_id":  int64(state.Config.LatestAggregatorRoundID),
				"latest_transmitter":          state.Config.LatestTransmitter[:],
				"config_count":                int64(state.Config.ConfigCount),
				"latest_config_digest":        state.Config.LatestConfigDigest[:],
				"latest_config_block_number":  uint64ToBeBytes(state.Config.LatestConfigBlockNumber),
				"billing": map[string]interface{}{
					"observation_payment": int64(state.Config.Billing.ObservationPayment),
				},
			},
			"oracles":          formatOracles(state.Oracles),
			"leftover_payment": formatLeftovers(state.LeftoverPayments),
			"transmissions":    state.Transmissions[:],
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

func MakeTransmissionMapping(
	envelope TransmissionEnvelope,
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	answer := envelope.Answer
	out := map[string]interface{}{
		"block_number": uint64ToBeBytes(envelope.BlockNumber),
		"answer": map[string]interface{}{
			"data":      answer.Data.Bytes(),
			"timestamp": int64(answer.Timestamp),
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

func MakeSimplifiedConfigSetMapping(
	envelope StateEnvelope,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	state := envelope.State
	offchainConfig, err := parseOffchainConfig(state.Config.OffchainConfig.Raw[:state.Config.OffchainConfig.Len])
	if err != nil {
		return nil, fmt.Errorf("failed to parse OffchainConfig blob from the program state: %w", err)
	}
	signers, err := json.Marshal(extractSigners(state.Oracles))
	if err != nil {
		return nil, fmt.Errorf("failed to parse signers: %w", err)
	}
	transmitters, err := json.Marshal(extractTransmitters(state.Oracles))
	if err != nil {
		return nil, fmt.Errorf("failed to parse transmitters: %w", err)
	}
	s, err := json.Marshal(int32ArrToInt64Arr(offchainConfig.S))
	if err != nil {
		return nil, fmt.Errorf("failed to parse s: %w", err)
	}

	oracles, err := createConfigSetSimplifiedOracles(offchainConfig.OffchainPublicKeys, offchainConfig.PeerIds, state.Oracles)
	if err != nil {
		return nil, fmt.Errorf("failed to oracles s: %w", err)
	}

	out := map[string]interface{}{
		"config_digest":      base64.StdEncoding.EncodeToString(state.Config.LatestConfigDigest[:]),
		"block_number":       uint64ToBeBytes(envelope.BlockNumber),
		"signers":            string(signers),
		"transmitters":       string(transmitters),
		"f":                  int32(state.Config.F),
		"delta_progress":     uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds),
		"delta_resend":       uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds),
		"delta_round":        uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds),
		"delta_grace":        uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds),
		"delta_stage":        uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds),
		"r_max":              int64(offchainConfig.RMax),
		"s":                  string(s),
		"oracles":            string(oracles),
		"feed_state_account": base64.StdEncoding.EncodeToString(feedConfig.StateAccount[:]),
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
	out := []interface{}{}
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		oracle := oracles.Raw[i]
		out = append(out, oracle.Signer.Key[:])
	}
	return out
}

func extractTransmitters(oracles pkgSolana.Oracles) []interface{} {
	out := []interface{}{}
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		oracle := oracles.Raw[i]
		out = append(out, oracle.Transmitter.Bytes())
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
	out := []interface{}{}
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		oracle := oracles.Raw[i]
		out = append(out, map[string]interface{}{
			"transmitter": oracle.Transmitter[:],
			"signer": map[string]interface{}{
				"key": oracle.Signer.Key[:],
			},
			"payee":         oracle.Payee[:],
			"from_round_id": int64(oracle.FromRoundID),
			"payment":       uint64ToBeBytes(oracle.Payment),
		})
	}
	return out
}

func formatLeftovers(leftovers pkgSolana.LeftoverPayments) []interface{} {
	out := []interface{}{}
	var i uint64
	for i = 0; i < leftovers.Len; i++ {
		leftover := leftovers.Raw[i]
		out = append(out, map[string]interface{}{
			"payee":  leftover.Payee[:],
			"amount": uint64ToBeBytes(leftover.Amount),
		})
	}
	return out
}

func int32ArrToInt64Arr(in []uint32) []int64 {
	out := []int64{}
	for _, i := range in {
		out = append(out, int64(i))
	}
	return out
}

func createConfigSetSimplifiedOracles(offchainPublicKeys [][]byte, peerId []string, oracles pkgSolana.Oracles) ([]byte, error) {
	if len(offchainPublicKeys) != len(peerId) && oracles.Len != uint64(len(peerId)) {
		return nil, fmt.Errorf("length missmatch len(offchainPublicKeys)=%d , oracles.Len=%d, len(peerId)=%d", len(offchainPublicKeys), oracles.Len, len(peerId))
	}
	var out []interface{}
	var i uint64
	for i = 0; i < oracles.Len; i++ {
		out = append(out, map[string]interface{}{
			"transmitter":         oracles.Raw[i].Transmitter,
			"peer_id":             peerId[i],
			"offchain_public_key": offchainPublicKeys[i],
		})
	}
	s, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("failed to parse oracles: %w", err)
	}
	return s, nil
}
