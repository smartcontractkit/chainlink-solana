package monitoring

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapping(t *testing.T) {
	t.Run("MakeConfigSetMapping", func(t *testing.T) {
		state, offchainConfig, numericalMedianOffchainConfig, err := generateState()
		require.NoError(t, err)
		envelope := StateEnvelope{
			State:       state,
			BlockNumber: rand.Uint64(),
		}
		solanaConfig := generateSolanaConfig()
		feedConfig := generateFeedConfig()

		output := []byte{}
		mapping, err := MakeConfigSetMapping(envelope, solanaConfig, feedConfig)
		require.NoError(t, err)
		serialized, err := configSetCodec.BinaryFromNative(output, mapping)
		require.NoError(t, err)
		deserialized, _, err := configSetCodec.NativeFromBinary(serialized)
		require.NoError(t, err)

		configSet, ok := deserialized.(map[string]interface{})
		require.True(t, ok, "config_set should be a map")
		require.Equal(t, configSet["block_number"], uint64ToBeBytes(envelope.BlockNumber), "config_set.block_number")

		contractConfig, ok := configSet["contract_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, contractConfig["config_digest"], state.Config.LatestConfigDigest[:], "contract_config.config_digest")
		require.Equal(t, contractConfig["config_count"], int64(state.Config.ConfigCount), "contract_config.config_count")
		require.Equal(t, contractConfig["signers"], extractSigners(state.Oracles), "contract_config.signers")
		require.Equal(t, contractConfig["transmitters"], extractTransmitters(state.Oracles), "contract_config.transmitters")
		require.Equal(t, contractConfig["f"], int32(state.Config.F), "contract_config.F")

		onchainConfigUnion, ok := contractConfig["onchain_config"].(map[string]interface{})
		require.True(t, ok)
		onchainConfig, ok := onchainConfigUnion["link.chain.ocr2.ocr2_numerical_median_onchain_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, onchainConfig["min"], state.Config.MinAnswer.BigInt().Bytes())
		require.Equal(t, onchainConfig["max"], state.Config.MaxAnswer.BigInt().Bytes())

		require.Equal(t, contractConfig["offchain_config_version"], uint64ToBeBytes(state.Config.OffchainConfig.Version))

		decodedOffchainConfigUnion, ok := contractConfig["offchain_config"].(map[string]interface{})
		require.True(t, ok)
		decodedOffchainConfig, ok := decodedOffchainConfigUnion["link.chain.ocr2.ocr2_offchain_config"].(map[string]interface{})
		require.True(t, ok)

		require.Equal(t, decodedOffchainConfig["delta_progress_nanoseconds"], uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds))
		require.Equal(t, decodedOffchainConfig["delta_resend_nanoseconds"], uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds))
		require.Equal(t, decodedOffchainConfig["delta_round_nanoseconds"], uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds))
		require.Equal(t, decodedOffchainConfig["delta_grace_nanoseconds"], uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds))
		require.Equal(t, decodedOffchainConfig["delta_stage_nanoseconds"], uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds))
		require.Equal(t, decodedOffchainConfig["r_max"], int64(offchainConfig.RMax))

		s, ok := decodedOffchainConfig["s"].([]interface{})
		require.True(t, ok)
		require.Equal(t, interfaceArrToUint32Arr(s), int32ArrToInt64Arr(offchainConfig.S))

		offchainPublicKeys, ok := decodedOffchainConfig["offchain_public_keys"].([]interface{})
		require.True(t, ok)
		require.Equal(t, interfaceArrToBytesArr(offchainPublicKeys), offchainConfig.OffchainPublicKeys)

		peerIDs, ok := decodedOffchainConfig["peer_ids"].([]interface{})
		require.True(t, ok)
		require.Equal(t, interfaceArrToStringArr(peerIDs), offchainConfig.PeerIds)

		reportingPluginConfigUnion, ok := decodedOffchainConfig["reporting_plugin_config"].(map[string]interface{})
		require.True(t, ok)
		reportingPluginConfig, ok := reportingPluginConfigUnion["link.chain.ocr2.ocr2_numerical_median_offchain_config"].(map[string]interface{})
		require.True(t, ok)

		require.Equal(t, reportingPluginConfig["alpha_report_infinite"], numericalMedianOffchainConfig.AlphaReportInfinite)
		require.Equal(t, reportingPluginConfig["alpha_report_ppb"], uint64ToBeBytes(numericalMedianOffchainConfig.AlphaReportPpb))
		require.Equal(t, reportingPluginConfig["alpha_accept_infinite"], numericalMedianOffchainConfig.AlphaAcceptInfinite)
		require.Equal(t, reportingPluginConfig["alpha_accept_ppb"], uint64ToBeBytes(numericalMedianOffchainConfig.AlphaAcceptPpb))
		require.Equal(t, reportingPluginConfig["delta_c_nanoseconds"], uint64ToBeBytes(numericalMedianOffchainConfig.DeltaCNanoseconds))

		require.Equal(t, decodedOffchainConfig["max_duration_query_nanoseconds"], uint64ToBeBytes(offchainConfig.MaxDurationQueryNanoseconds))
		require.Equal(t, decodedOffchainConfig["max_duration_observation_nanoseconds"], uint64ToBeBytes(offchainConfig.MaxDurationObservationNanoseconds))
		require.Equal(t, decodedOffchainConfig["max_duration_report_nanoseconds"], uint64ToBeBytes(offchainConfig.MaxDurationReportNanoseconds))
		require.Equal(t, decodedOffchainConfig["max_duration_should_accept_finalized_report_nanoseconds"], uint64ToBeBytes(offchainConfig.MaxDurationShouldAcceptFinalizedReportNanoseconds))
		require.Equal(t, decodedOffchainConfig["max_duration_should_transmit_accepted_report_nanoseconds"], uint64ToBeBytes(offchainConfig.MaxDurationShouldTransmitAcceptedReportNanoseconds))

		sharedSecredEncryptions, ok := decodedOffchainConfig["shared_secret_encryptions"].(map[string]interface{})
		require.True(t, ok)

		require.Equal(t, sharedSecredEncryptions["diffie_hellman_point"], offchainConfig.SharedSecretEncryptions.DiffieHellmanPoint)
		require.Equal(t, sharedSecredEncryptions["shared_secret_hash"], offchainConfig.SharedSecretEncryptions.SharedSecretHash)

		encryptions, ok := sharedSecredEncryptions["encryptions"].([]interface{})
		require.True(t, ok)
		require.Equal(t, interfaceArrToBytesArr(encryptions), offchainConfig.SharedSecretEncryptions.Encryptions)

		solanaProgramState, ok := configSet["solana_program_state"].(map[string]interface{})
		require.True(t, ok)

		require.Equal(t, solanaProgramState["account_discriminator"], state.AccountDiscriminator[:])
		require.Equal(t, solanaProgramState["version"], int32(state.Version))
		require.Equal(t, solanaProgramState["nonce"], int32(state.Nonce))

		config, ok := solanaProgramState["config"].(map[string]interface{})
		require.True(t, ok, "solana_program_state.config should be a map")
		require.Equal(t, config["owner"], state.Config.Owner.Bytes())
		require.Equal(t, config["token_mint"], state.Config.TokenMint.Bytes())
		require.Equal(t, config["token_vault"], state.Config.TokenVault.Bytes())
		require.Equal(t, config["requester_access_controller"], state.Config.RequesterAccessController.Bytes())
		require.Equal(t, config["billing_access_controller"], state.Config.BillingAccessController.Bytes())
		require.Equal(t, config["min_answer"], state.Config.MinAnswer.BigInt().Bytes())
		require.Equal(t, config["max_answer"], state.Config.MaxAnswer.BigInt().Bytes())
		require.Equal(t, config["description"], state.Config.Description[:])
		require.Equal(t, config["decimals"], int32(state.Config.Decimals))
		require.Equal(t, config["f"], int32(state.Config.F))
		require.Equal(t, config["round"], int32(state.Config.Round))
		require.Equal(t, config["epoch"], int64(state.Config.Epoch))
		require.Equal(t, config["latest_aggregator_round_id"], int64(state.Config.LatestAggregatorRoundID))
		require.Equal(t, config["latest_transmitter"], state.Config.LatestTransmitter.Bytes())
		require.Equal(t, config["config_count"], int64(state.Config.ConfigCount))
		require.Equal(t, config["latest_config_digest"], state.Config.LatestConfigDigest[:])
		require.Equal(t, config["latest_config_block_number"], uint64ToBeBytes(state.Config.LatestConfigBlockNumber))

		billing, ok := config["billing"].(map[string]interface{})
		require.True(t, ok, "billing is a map")
		require.Equal(t, billing["observation_payment"], int64(state.Config.Billing.ObservationPayment))

		require.Equal(t, config["validator"], state.Config.Validator.Bytes())
		require.Equal(t, config["flagging_threshold"], int64(state.Config.FlaggingThreshold))

		oracles, ok := solanaProgramState["oracles"].([]interface{})
		require.True(t, ok, "oracles is an array")
		require.Equal(t, len(oracles), int(state.Oracles.Len))
		for i := 0; i < len(oracles); i++ {
			oracle, ok := oracles[i].(map[string]interface{})
			require.True(t, ok, "oracle is a map")
			require.Equal(t, oracle["transmitter"], state.Oracles.Raw[i].Transmitter.Bytes())

			signer, ok := oracle["signer"].(map[string]interface{})
			require.True(t, ok, "signer needs to be a map")
			require.Equal(t, signer["key"], state.Oracles.Raw[i].Signer.Key[:])

			require.Equal(t, oracle["payee"], state.Oracles.Raw[i].Payee.Bytes())
			require.Equal(t, oracle["from_round_id"], int64(state.Oracles.Raw[i].FromRoundID))
			require.Equal(t, oracle["payment"], uint64ToBeBytes(state.Oracles.Raw[i].Payment))
		}

		leftovers, ok := solanaProgramState["leftover_payment"].([]interface{})
		require.True(t, ok, "leftover_payment is an array")
		for i := 0; i < len(leftovers); i++ {
			leftover, ok := leftovers[i].(map[string]interface{})
			require.True(t, ok, "leftover_payment is a map")
			require.Equal(t, leftover["payee"], state.LeftoverPayments.Raw[i].Payee.Bytes())
			require.Equal(t, leftover["amount"], uint64ToBeBytes(state.LeftoverPayments.Raw[i].Amount))
		}

		require.Equal(t, solanaProgramState["transmissions"], state.Transmissions.Bytes())

		solanaChainConfig, ok := configSet["solana_chain_config"].(map[string]interface{})
		require.True(t, ok, "config_set.solana_chain_config should be a map")
		require.Equal(t, solanaChainConfig["network_name"], solanaConfig.NetworkName)
		require.Equal(t, solanaChainConfig["network_id"], solanaConfig.NetworkID)
		require.Equal(t, solanaChainConfig["chain_id"], solanaConfig.ChainID)

		decodedFeedConfig, ok := configSet["feed_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, decodedFeedConfig["feed_name"], feedConfig.FeedName)
		require.Equal(t, decodedFeedConfig["feed_path"], feedConfig.FeedPath)
		require.Equal(t, decodedFeedConfig["symbol"], feedConfig.Symbol)
		require.Equal(t, decodedFeedConfig["heartbeat_sec"], int64(feedConfig.HeartbeatSec))
		require.Equal(t, decodedFeedConfig["contract_type"], feedConfig.ContractType)
		require.Equal(t, decodedFeedConfig["contract_status"], feedConfig.ContractStatus)
		require.Equal(t, decodedFeedConfig["contract_address"], feedConfig.ContractAddress.Bytes())
		require.Equal(t, decodedFeedConfig["transmissions_account"], feedConfig.TransmissionsAccount.Bytes())
		require.Equal(t, decodedFeedConfig["state_account"], feedConfig.StateAccount.Bytes())
	})

	t.Run("MakeTransmissionMapping", func(t *testing.T) {
		initial := generateTransmissionEnvelope()
		solanaConfig := generateSolanaConfig()
		feedConfig := generateFeedConfig()

		mapping, err := MakeTransmissionMapping(initial, solanaConfig, feedConfig)
		require.NoError(t, err)
		output := []byte{}
		serialized, err := transmissionCodec.BinaryFromNative(output, mapping)
		require.NoError(t, err)
		deserialized, _, err := transmissionCodec.NativeFromBinary(serialized)
		require.NoError(t, err)

		transmission, ok := deserialized.(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, transmission["block_number"], uint64ToBeBytes(initial.BlockNumber))

		answer, ok := transmission["answer"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, answer["data"], initial.Answer.Data.Bytes())
		require.Equal(t, answer["timestamp"].(int64), int64(initial.Answer.Timestamp))

		solanaChainConfig, ok := transmission["solana_chain_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, solanaChainConfig["network_name"], solanaConfig.NetworkName)
		require.Equal(t, solanaChainConfig["network_id"], solanaConfig.NetworkID)
		require.Equal(t, solanaChainConfig["chain_id"], solanaConfig.ChainID)

		decodedFeedConfig, ok := transmission["feed_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, decodedFeedConfig["feed_name"], feedConfig.FeedName)
		require.Equal(t, decodedFeedConfig["feed_path"], feedConfig.FeedPath)
		require.Equal(t, decodedFeedConfig["symbol"], feedConfig.Symbol)
		require.Equal(t, decodedFeedConfig["heartbeat_sec"], int64(feedConfig.HeartbeatSec))
		require.Equal(t, decodedFeedConfig["contract_type"], feedConfig.ContractType)
		require.Equal(t, decodedFeedConfig["contract_status"], feedConfig.ContractStatus)
		require.Equal(t, decodedFeedConfig["contract_address"], feedConfig.ContractAddress.Bytes())
		require.Equal(t, decodedFeedConfig["transmissions_account"], feedConfig.TransmissionsAccount.Bytes())
		require.Equal(t, decodedFeedConfig["state_account"], feedConfig.StateAccount.Bytes())
	})
}

// Helpers

func interfaceArrToUint32Arr(in []interface{}) []int64 {
	out := []int64{}
	for _, i := range in {
		out = append(out, i.(int64))
	}
	return out
}

func interfaceArrToBytesArr(in []interface{}) [][]byte {
	out := [][]byte{}
	for _, i := range in {
		out = append(out, i.([]byte))
	}
	return out
}

func interfaceArrToStringArr(in []interface{}) []string {
	out := []string{}
	for _, i := range in {
		out = append(out, i.(string))
	}
	return out
}
