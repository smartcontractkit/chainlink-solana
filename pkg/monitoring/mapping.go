package monitoring

import (
	"encoding/binary"
	"encoding/hex"
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
			"account_discriminator": state.AccountDiscriminator[:],
			"nonce":                 int32(state.Nonce),
			"config": map[string]interface{}{
				"version":                     int32(state.Config.Version),
				"owner":                       state.Config.Owner[:],
				"token_mint":                  state.Config.TokenMint[:],
				"token_vault":                 state.Config.TokenVault[:],
				"requester_access_controller": state.Config.RequesterAccessController[:],
				"billing_access_controller":   state.Config.BillingAccessController[:],
				"min_answer":                  state.Config.MinAnswer.BigInt().Bytes(),
				"max_answer":                  state.Config.MaxAnswer.BigInt().Bytes(),
				"decimals":                    int32(state.Config.Decimals),
				"description":                 state.Config.Description[:],
				"f":                           int32(state.Config.F),
				"config_count":                int32(state.Config.ConfigCount),
				"latest_config_digest":        state.Config.LatestConfigDigest[:],
				"latest_config_block_number":  int64(state.Config.LatestConfigBlockNumber),
				"latest_aggregator_round_id":  int32(state.Config.LatestAggregatorRoundID),
				"epoch":                       int32(state.Config.Epoch),
				"round":                       int32(state.Config.Round),
				"billing": map[string]interface{}{
					"observation_payment": int32(state.Config.Billing.ObservationPayment),
				},
				"validator":          state.Config.Validator[:],
				"flagging_threshold": int(state.Config.FlaggingThreshold),
			},
			"oracles":              formatOracles(state.Oracles),
			"leftover_payment":     formatLeftovers(state.LeftoverPayment, state.LeftoverPaymentLen),
			"leftover_payment_len": int32(state.LeftoverPaymentLen),
			"transmissions":        state.Transmissions[:],
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

func MakeTelemetryConfigSetMapping(
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
	signers, err := convertToJson(extractSigners(state.Oracles))
	if err != nil {
		return nil, fmt.Errorf("failed to parse signers: %w", err)
	}
	transmitters, err := convertToJson(extractTransmitters(state.Oracles))
	if err != nil {
		return nil, fmt.Errorf("failed to parse transmitters: %w", err)
	}
	s, err := convertToJson(int32ArrToInt64Arr(offchainConfig.S))
	if err != nil {
		return nil, fmt.Errorf("failed to parse s: %w", err)
	}

	oracles, err := createTelemetryOracles(offchainConfig.OffchainPublicKeys, offchainConfig.PeerIds, state.Oracles)
	if err != nil {
		return nil, fmt.Errorf("failed to oracles s: %w", err)
	}

	out := map[string]interface{}{
		"config_digest":        hex.EncodeToString(state.Config.LatestConfigDigest[:]),
		"block_number":         uint64ToBeBytes(envelope.BlockNumber),
		"signers":              signers,
		"transmitters":         transmitters,
		"f":                    int32(state.Config.F),
		"delta_progress":       uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds),
		"delta_resend":         uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds),
		"delta_round":          uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds),
		"delta_grace":          uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds),
		"delta_stage":          uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds),
		"delta_c":              uint64ToBeBytes(numericalMedianOffchainConfig.DeltaCNanoseconds),
		"alpha_ppb":            uint64ToBeBytes(numericalMedianOffchainConfig.AlphaAcceptPpb),
		"r_max":                int64(offchainConfig.RMax),
		"s":                    s,
		"oracles":              oracles,
		"feed_contract_status": feedConfig.ContractStatus,
	}
	return out, nil

}

// Helpers

func uint64ToBeBytes(input uint64) []byte {
	buf := make([]byte, 8)
	_ = binary.PutUvarint(buf, input)
	return buf
}

func extractSigners(oracles pkgSolana.Oracles) []interface{} {
	out := []interface{}{}
	var i uint8
	for i = 0; i < oracles.Len; i++ {
		oracle := oracles.Raw[i]
		out = append(out, oracle.Signer.Key[:])
	}
	return out
}

func extractTransmitters(oracles pkgSolana.Oracles) []interface{} {
	out := []interface{}{}
	var i uint8
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
	var i uint8
	for i = 0; i < oracles.Len; i++ {
		oracle := oracles.Raw[i]
		out = append(out, map[string]interface{}{
			"transmitter": oracle.Transmitter[:],
			"signer": map[string]interface{}{
				"key": oracle.Signer.Key[:],
			},
			"payee":          oracle.Payee[:],
			"proposed_payee": oracle.ProposedPayee[:],
			"payment":        int64(oracle.Payment),
			"from_round_id":  int32(oracle.FromRoundID),
		})
	}
	return out
}

func formatLeftovers(leftovers [19]pkgSolana.LeftoverPayment, leftoversLen uint8) []interface{} {
	out := []interface{}{}
	var i uint8
	for i = 0; i < leftoversLen && i < 19; i++ {
		leftover := leftovers[i]
		out = append(out, map[string]interface{}{
			"payee":  leftover.Payee[:],
			"amount": int64(leftover.Amount),
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

func convertToJson(i interface{}) (string, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func createTelemetryOracles(offchainPublicKeys [][]byte, peerId []string, oracles pkgSolana.Oracles) (interface{}, error) {
	if len(offchainPublicKeys) != len(peerId) && oracles.Len != uint8(len(peerId)) {
		return nil, fmt.Errorf("length missmatch len(offchainPublicKeys)=%d , oracles.Len=%d, len(peerId)=%d", len(offchainPublicKeys), oracles.Len, len(peerId))
	}
	var out []interface{}
	var i uint8
	for i = 0; i < oracles.Len; i++ {
		out = append(out, map[string]interface{}{
			"transmitter":         oracles.Raw[i].Transmitter,
			"peer_id":             peerId[i],
			"offchain_public_key": offchainPublicKeys[i],
		})
	}
	s, err := convertToJson(out)
	if err != nil {
		return nil, fmt.Errorf("failed to parse oracles: %w", err)
	}
	return s, nil
}
