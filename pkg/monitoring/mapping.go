package monitoring

import (
	"encoding/binary"
	"fmt"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/pb"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"google.golang.org/protobuf/proto"
)

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
		out = append(out, oracle.Transmitter[:])
	}
	return out
}

func parseOffchainConfig(buf []byte) (pb.OffchainConfigProto, error) {
	config := pb.OffchainConfigProto{}
	err := proto.Unmarshal(buf, &config)
	return config, err
}

func parseNumericalMedianOffchainConfig(buf []byte) (pb.NumericalMedianConfigProto, error) {
	config := pb.NumericalMedianConfigProto{}
	err := proto.Unmarshal(buf, &config)
	return config, err
}

func formatOracles(oracles []pkgSolana.Oracle) []interface{} {
	out := []interface{}{}
	for _, oracle := range oracles {
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

func formatLeftovers(leftovers []pkgSolana.LeftoverPayment) []interface{} {
	out := []interface{}{}
	for _, leftover := range leftovers {
		out = append(out, map[string]interface{}{
			"payee":  leftover.Payee[:],
			"amount": int64(leftover.Amount),
		})
	}
	return out
}

func MakeConfigSetMapping(
	envelope StateEnvelope,
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	state := envelope.State
	offchainConfig, err := parseOffchainConfig(state.Config.OffchainConfig.Raw[:state.Config.OffchainConfig.Len])
	if err != nil {
		return nil, fmt.Errorf("failed to parse OffchainConfig blob from the program state, err: %w", err)
	}
	numericalMedianOffchainConfig, err := parseNumericalMedianOffchainConfig(offchainConfig.ReportingPluginConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ReportingPluginConfig from OffchainConfig, err: %w", err)
	}
	out := map[string]interface{}{
		"block_number": uint64ToBeBytes(envelope.BlockNumber),
		"contract_config": map[string]interface{}{
			"config_digest": state.Config.LatestConfigDigest[:],
			"signers":       extractSigners(state.Oracles),
			"transmitters":  extractTransmitters(state.Oracles),
			"f":             int64(state.Config.F),
			"onchain_config": map[string]interface{}{
				"min": state.Config.MinAnswer.BigInt().Bytes(),
				"max": state.Config.MaxAnswer.BigInt().Bytes(),
			},
			"offchain_config_version": uint64ToBeBytes(state.Config.OffchainConfig.Version),
			"offchain_config": map[string]interface{}{
				"delta_progress_nanoseconds": uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds),
				"delta_resend_nanoseconds":   uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds),
				"delta_round_nanoseconds":    uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds),
				"delta_grace_nanoseconds":    uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds),
				"delta_stage_nanoseconds":    uint64ToBeBytes(offchainConfig.DeltastateNanoseconds),
				"r_max":                      int64(offchainConfig.RMax),
				"s":                          offchainConfig.S,
				"offchain_public_keys":       offchainConfig.OffchainPublicKeys,
				"peer_ids":                   offchainConfig.PeerIds,
				"reporting_plugin_config": map[string]interface{}{
					"alpha_report_infinite": numericalMedianOffchainConfig.AlphaReportInfinite,
					"alpha_report_ppb":      uint64ToBeBytes(numericalMedianOffchainConfig.AlphaReportPPB),
					"alpha_accept_infinite": numericalMedianOffchainConfig.AlphaAcceptInfinite,
					"alpha_accept_ppb":      uint64ToBeBytes(numericalMedianOffchainConfig.AlphaAcceptPPB),
					"delta_c_nanoseconds":   uint64ToBeBytes(numericalMedianOffchainConfig.DeltaCNanoseconds),
				},
				"max_duration_query_nanoseconds":                           uint64ToBeBytes(offchainConfig.MaxDurationQueryNanoseconds),
				"max_duration_observation_nanoseconds":                     uint64ToBeBytes(offchainConfig.MaxDurationObservationNanosecods),
				"max_duration_report_nanoseconds":                          uint64ToBeBytes(offchainConfig.MaxDurationReportNanoseconds),
				"max_duration_should_accept_finalized_report_nanoseconds":  uint64ToBeBytes(offchainConfig.MaxDurationShouldAcceptFinalizedReportNanoseconds),
				"max_duration_should_transmit_accepted_report_nanoseconds": uint64ToBeBytes(offchainConfig.MaxDurationShouldTransmitAcceptedReportNanoseconds),
				"shared_secret_encryptions": map[string]interface{}{
					"diffie_hellman_point": offchainConfig.SharedSecretEncryptionsProto.DiffieHellmanPoint,
					"shared_secret_hash":   offchainConfig.SharedSecretEncryptionsProto.SharedSecretHash,
					"encryptions":          offchainConfig.SharedSecretEncryptionsProto.Exceptions,
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
			"leftover_payment":     formatLeftovers(state.Oracles),
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
	lastStateSnapshot StateEnvelope,
	solanaConfig SolanaConfig,
	feedConfig FeedConfig,
) (map[string]interface{}, error) {
	answer := envelope.Answer
	out := map[string]interface{}{
		"block_number": uint64ToBeBytes(envelope.BlockNumber),
		"answer": map[string]interface{}{
			"value":     answer.Data.Bytes(),
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
