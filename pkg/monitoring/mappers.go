package monitoring

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/event"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

type Mapper func(interface{}, SolanaConfig, SolanaFeedConfig) (map[string]interface{}, error)

func StateMapper(raw interface{}, _ SolanaConfig, feedConfig SolanaFeedConfig) (map[string]interface{}, error) {
	account, isStateAccount := raw.(StateAccount)
	if !isStateAccount {
		return nil, fmt.Errorf("expected input of type StateAccount but got '%T'", raw)
	}
	sharedSecredEncryptions := map[string]interface{}{
		"diffie_hellman_point": []byte{},
		"shared_secret_hash":   []byte{},
		"encryptions":          []byte{},
	}
	if account.OffchainConfig.SharedSecretEncryptions != nil {
		sharedSecredEncryptions = map[string]interface{}{
			"diffie_hellman_point": account.OffchainConfig.SharedSecretEncryptions.DiffieHellmanPoint,
			"shared_secret_hash":   account.OffchainConfig.SharedSecretEncryptions.SharedSecretHash,
			"encryptions":          account.OffchainConfig.SharedSecretEncryptions.Encryptions,
		}
	}
	oracles, err := oraclesMapper(account.State.Oracles)
	if err != nil {
		return nil, fmt.Errorf("failed to extract oracles from the state account: %w", err)
	}
	out := map[string]interface{}{
		"account_public_key": feedConfig.StateAccount[:],

		"slot":       uint64ToBeBytes(account.Slot),
		"lamports":   uint64ToBeBytes(account.Lamports),
		"owner":      account.Owner[:],
		"executable": account.Executable,
		"rent_epoch": uint64ToBeBytes(account.RentEpoch),

		"state": map[string]interface{}{
			"account_discriminator": account.State.AccountDiscriminator[:8],
			"version":               int32(account.State.Version),
			"nonce":                 int32(account.State.Nonce),
			"transmissions":         account.State.Transmissions[:],
			"config": map[string]interface{}{
				"owner":                       account.State.Config.Owner[:],
				"proposed_owner":              account.State.Config.ProposedOwner[:],
				"token_mint":                  account.State.Config.TokenMint[:],
				"token_vault":                 account.State.Config.TokenVault[:],
				"requester_access_controller": account.State.Config.RequesterAccessController[:],
				"billing_access_controller":   account.State.Config.BillingAccessController[:],
				"min_answer":                  account.State.Config.MinAnswer.BigInt().Bytes(),
				"max_answer":                  account.State.Config.MaxAnswer.BigInt().Bytes(),
				"f":                           int32(account.State.Config.F),
				"round":                       int32(account.State.Config.Round),
				"epoch":                       int64(account.State.Config.Epoch),
				"latest_aggregator_round_id":  int64(account.State.Config.LatestAggregatorRoundID),
				"latest_transmitter":          account.State.Config.LatestTransmitter[:],
				"config_count":                int64(account.State.Config.ConfigCount),
				"latest_config_digest":        account.State.Config.LatestConfigDigest[:],
				"latest_config_block_number":  uint64ToBeBytes(account.State.Config.LatestConfigBlockNumber),
				"billing": map[string]interface{}{
					"observation_payment":  int64(account.State.Config.Billing.ObservationPayment),
					"transmission_payment": int64(account.State.Config.Billing.TransmissionPayment),
				},
			},

			"offchain_config_version": uint64ToBeBytes(account.State.OffchainConfig.Version),
			"offchain_config": map[string]interface{}{
				"link.chain.ocr2.ocr2_offchain_config": map[string]interface{}{
					"delta_progress_nanoseconds": uint64ToBeBytes(account.OffchainConfig.DeltaProgressNanoseconds),
					"delta_resend_nanoseconds":   uint64ToBeBytes(account.OffchainConfig.DeltaResendNanoseconds),
					"delta_round_nanoseconds":    uint64ToBeBytes(account.OffchainConfig.DeltaRoundNanoseconds),
					"delta_grace_nanoseconds":    uint64ToBeBytes(account.OffchainConfig.DeltaGraceNanoseconds),
					"delta_stage_nanoseconds":    uint64ToBeBytes(account.OffchainConfig.DeltaStageNanoseconds),
					"r_max":                      int64(account.OffchainConfig.RMax),
					"s":                          int32ArrToInt64Arr(account.OffchainConfig.S),
					"offchain_public_keys":       account.OffchainConfig.OffchainPublicKeys,
					"peer_ids":                   account.OffchainConfig.PeerIds,
					"reporting_plugin_config": map[string]interface{}{
						"link.chain.ocr2.ocr2_numerical_median_offchain_config": map[string]interface{}{
							"alpha_report_infinite": account.NumericalMedianConfig.AlphaReportInfinite,
							"alpha_report_ppb":      uint64ToBeBytes(account.NumericalMedianConfig.AlphaReportPpb),
							"alpha_accept_infinite": account.NumericalMedianConfig.AlphaAcceptInfinite,
							"alpha_accept_ppb":      uint64ToBeBytes(account.NumericalMedianConfig.AlphaAcceptPpb),
							"delta_c_nanoseconds":   uint64ToBeBytes(account.NumericalMedianConfig.DeltaCNanoseconds),
						},
					},
					"max_duration_query_nanoseconds":                           uint64ToBeBytes(account.OffchainConfig.MaxDurationQueryNanoseconds),
					"max_duration_observation_nanoseconds":                     uint64ToBeBytes(account.OffchainConfig.MaxDurationObservationNanoseconds),
					"max_duration_report_nanoseconds":                          uint64ToBeBytes(account.OffchainConfig.MaxDurationReportNanoseconds),
					"max_duration_should_accept_finalized_report_nanoseconds":  uint64ToBeBytes(account.OffchainConfig.MaxDurationShouldAcceptFinalizedReportNanoseconds),
					"max_duration_should_transmit_accepted_report_nanoseconds": uint64ToBeBytes(account.OffchainConfig.MaxDurationShouldTransmitAcceptedReportNanoseconds),
					"shared_secret_encryptions":                                sharedSecredEncryptions,
				},
			},

			"oracles": oracles,
		},
	}

	return out, nil
}

func TransmissionsMapper(raw interface{}, _ SolanaConfig, feedConfig SolanaFeedConfig) (map[string]interface{}, error) {
	account, isTransmissionsAccount := raw.(TransmissionsAccount)
	if !isTransmissionsAccount {
		return nil, fmt.Errorf("expected input of type TransmissionsAccount but got '%T'", raw)
	}
	out := map[string]interface{}{
		"account_public_key": feedConfig.StateAccount[:],

		"slot":       uint64ToBeBytes(account.Slot),
		"lamports":   uint64ToBeBytes(account.Lamports),
		"owner":      account.Owner[:],
		"executable": account.Executable,
		"rent_epoch": uint64ToBeBytes(account.RentEpoch),

		"header": map[string]interface{}{
			"version":            int32(account.Header.Version),
			"state":              int32(account.Header.State),
			"owner":              account.Header.Owner[:],
			"proposed_owner":     account.Header.ProposedOwner[:],
			"writer":             account.Header.Writer[:],
			"description":        account.Header.Description[:],
			"decimals":           int32(account.Header.Decimals),
			"flagging_threshold": int64(account.Header.FlaggingThreshold),
			"latest_round_id":    int64(account.Header.LatestRoundID),
			"granularity":        int32(account.Header.Granularity),
			"live_length":        int64(account.Header.LiveLength),
			"live_cursor":        int64(account.Header.LiveCursor),
			"historical_cursor":  int64(account.Header.HistoricalCursor),
		},

		"transmission": map[string]interface{}{
			"slot":      uint64ToBeBytes(account.Transmission.Slot),
			"timestamp": int64(account.Transmission.Timestamp),
			"answer":    account.Transmission.Answer.BigInt().Bytes(),
		},
	}
	return out, nil
}

func LogMapper(raw interface{}, _ SolanaConfig, feedConfig SolanaFeedConfig) (map[string]interface{}, error) {
	logs, isLogs := raw.(Logs)
	if !isLogs {
		return nil, fmt.Errorf("expected input of type Logs but got '%T'", raw)
	}
	events, err := eventsMapper(logs.Events)
	if err != nil {
		return nil, fmt.Errorf("failed to map events from logs: %w", err)
	}
	out := map[string]interface{}{
		"program_public_key": feedConfig.ContractAddress[:],

		"slot":      uint64ToBeBytes(logs.Slot),
		"signature": logs.Signature[:],
		"err":       logs.Err,

		"events": events,
	}
	return out, nil
}

func BlockMapper(raw interface{}, _ SolanaConfig, feedConfig SolanaFeedConfig) (map[string]interface{}, error) {
	block, isBlock := raw.(Block)
	if !isBlock {
		return nil, fmt.Errorf("expected input of type Block but got '%T'", raw)
	}
	transactions := []interface{}{}
	for _, tx := range block.Transactions {
		transactions = append(transactions, map[string]interface{}{
			"data": tx.Transaction.GetBinary(),
			"meta": map[string]interface{}{
				"err":                 fmt.Sprintf("%s", tx.Meta.Err),
				"fee":                 uint64ToBeBytes(tx.Meta.Fee),
				"pre_balances":        uint64ArrToBytesArr(tx.Meta.PreBalances),
				"post_balances":       uint64ArrToBytesArr(tx.Meta.PostBalances),
				"pre_token_balances":  mapTokenBalances(tx.Meta.PreTokenBalances),
				"post_token_balances": mapTokenBalances(tx.Meta.PostTokenBalances),
				"log_messages":        tx.Meta.LogMessages,
				"rewards":             mapRewards(tx.Meta.Rewards),
			},
		})
	}
	out := map[string]interface{}{
		"program_public_key": feedConfig.ContractAddress[:],
		"slot":               uint64ToBeBytes(block.Slot),
		"err":                block.Err,
		"blockhash":          block.Blockhash[:],
		"previous_blockhash": block.PreviousBlockhash[:],
		"parent_slot":        uint64ToBeBytes(block.ParentSlot),
		"block_time_sec":     uint64ToBeBytes(uint64(block.BlockTime.Unix())),
		"block_height":       uint64ToBeBytes(block.BlockHeight),
		"transactions":       transactions,
		"rewards":            mapRewards(block.Rewards),
	}
	return out, nil
}

// Helpers

func uint64ToBeBytes(input uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, input)
	return buf
}

func uint64ArrToBytesArr(xs []uint64) [][]byte {
	out := make([][]byte, len(xs))
	for i, x := range xs {
		out[i] = uint64ToBeBytes(x)
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

func oraclesMapper(rawOracles pkgSolana.Oracles) ([]interface{}, error) {
	oracles, err := rawOracles.Data()
	if err != nil {
		return nil, err
	}
	out := make([]interface{}, len(oracles))
	for i, oracle := range oracles {
		out[i] = map[string]interface{}{
			"transmitter": oracle.Transmitter[:],
			"signer": map[string]interface{}{
				"key": oracle.Signer.Key[:],
			},
			"payee":          oracle.Payee[:],
			"proposed_payee": oracle.ProposedPayee[:],
			"from_round_id":  int64(oracle.FromRoundID),
			"payment":        uint64ToBeBytes(oracle.Payment),
		}
	}
	return out, nil
}

func eventsMapper(events []interface{}) ([]interface{}, error) {
	out := []interface{}{}
	for _, rawEvent := range events {
		switch typed := rawEvent.(type) {
		case event.SetConfig:
			out = append(out, map[string]interface{}{
				"link.chain.ocr2.ocr2_event_set_config": map[string]interface{}{
					"config_digest": typed.ConfigDigest[:],
					"f":             int32(typed.F),
					"signers":       typed.Signers[:],
				},
			})
		case event.SetBilling:
			out = append(out, map[string]interface{}{
				"link.chain.ocr2.ocr2_event_set_billing": map[string]interface{}{
					"observation_payment_gjuels":  int64(typed.ObservationPaymentGJuels),
					"transmission_payment_gjuels": int64(typed.TransmissionPaymentGJuels),
				},
			})
		case event.RoundRequested:
			out = append(out, map[string]interface{}{
				"link.chain.ocr2.ocr2_event_round_requested": map[string]interface{}{
					"config_digest": typed.ConfigDigest[:],
					"requester":     typed.Requester[:],
					"epoch":         int64(typed.Epoch),
					"round":         int32(typed.Round),
				},
			})
		case event.NewTransmission:
			observers := make([]int, len(typed.Observers))
			for i, obs := range typed.Observers {
				observers[i] = int(obs)
			}
			out = append(out, map[string]interface{}{
				"link.chain.ocr2.ocr2_event_new_transmission": map[string]interface{}{
					"round_id":               int64(typed.RoundID),
					"config_digest":          typed.ConfigDigest[:],
					"answer":                 typed.Answer.BigInt().Bytes(),
					"transmitter":            int32(typed.Transmitter),
					"observations_timestamp": int64(typed.ObservationsTimestamp),
					"observer_count":         int32(typed.ObserverCount),
					"observers":              observers,
					"juels_per_lamport":      uint64ToBeBytes(typed.JuelsPerLamport),
					"reimbursement_gjuels":   uint64ToBeBytes(typed.ReimbursementGJuels),
				},
			})
		}
	}
	return out, nil
}

func mapTokenBalances(balances []rpc.TokenBalance) []interface{} {
	out := []interface{}{}
	for _, balance := range balances {
		amount := map[string]interface{}{}
		var uiAmount float64
		if balance.UiTokenAmount.UiAmount != nil {
			uiAmount = *balance.UiTokenAmount.UiAmount
		}
		if balance.UiTokenAmount != nil {
			amount["amount"] = balance.UiTokenAmount.Amount
			amount["decimals"] = int32(balance.UiTokenAmount.Decimals)
			amount["ui_amount"] = uiAmount
			amount["ui_amount_string"] = balance.UiTokenAmount.UiAmountString
		}
		out = append(out, map[string]interface{}{
			"account_index": int32(balance.AccountIndex),
			"owner":         balance.Owner.Bytes(),
			"mint":          balance.Mint.Bytes(),
			"amount":        amount,
		})
	}
	return out
}

func mapRewards(rawRewards []rpc.BlockReward) []interface{} {
	rewards := []interface{}{}
	for _, reward := range rawRewards {
		var commission uint8
		if reward.Commission != nil {
			commission = *reward.Commission
		}
		rewards = append(rewards, map[string]interface{}{
			"public_key":   reward.Pubkey.Bytes(),
			"lamports":     uint64ToBeBytes(uint64(reward.Lamports)),
			"post_balance": uint64ToBeBytes(reward.PostBalance),
			"reward_type":  string(reward.RewardType),
			"commission":   int32(commission),
		})
	}
	return rewards
}
