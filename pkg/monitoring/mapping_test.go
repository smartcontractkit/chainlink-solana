package monitoring

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func bigEndianBytesToUint64(buf []byte) uint64 {
	return 0
}

func TestMapping(t *testing.T) {
	t.Run("MakeConfigSetMapping", func(t *testing.T) {
		state := generateState()
		envelope := StateEnvelope{
			State:       state,
			BlockNumber: 100,
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
		require.Equal(t, configSet["block_number"], envelope.BlockNumber, "config_set.block_number")

		contractConfig, ok := configSet["contract_config"].(map[string]interface{})
		require.True(t, ok, "config_set.contract_config should be a map")
		require.Equal(contractConfig["config_digest"], state.Config.LatestConfigDigest[:], "contract_config.config_digest")
		require.Equal(contractConfig["signers"], extractSigners(state.Oracles), "contract_config.signers")
		require.Equal(contractConfig["transmitters"], extractTransmitters(state.Oracles), "contract_config.transmitters")
		require.Equal(contractConfig["f"], int64(state.Config.F), "contract_config.F")

		onchainConfig, ok := contractConfig["onchain_config"].(map[string]interface{})
		require.True(t, ok, "contract_config.onchain_config should be a map")
		require.Equal(t, onchainConfig["min"], state.Config.MinAnswer.BigInt().Bytes())
		require.Equal(t, onchainConfig["max"], state.Config.MaxAnswer.BigInt().Bytes())

		require.Equal(contractConfig["offchain_config_version"], uint64ToBeBytes(state.Config.OffchainConfig.Version))

		//require.Equal(t, state["account_discriminator"], initial.AccountDiscriminator[:])
		//require.Equal(t, state["nonce"], int32(initial.Nonce))
		//require.Equal(t, state["leftover_payment_len"], int32(initial.LeftoverPaymentLen))
		//require.Equal(t, state["transmissions"], initial.Tranmissions.Bytes())

		//config, ok := state["config"].(map[string]interface{})
		//require.True(t, ok, "config should be a map")
		//require.Equal(t, config["version"], int32(initial.Config.Version))
		//require.Equal(t, config["owner"], initial.Config.Owner.Bytes())
		//require.Equal(t, config["token_mint"], initial.Config.TokenMint.Bytes())
		//require.Equal(t, config["token_vault"], initial.Config.TokenVault.Bytes())
		//require.Equal(t, config["requester_access_controller"], initial.Config.RequesterAccessController.Bytes())
		//require.Equal(t, config["billing_access_controller"], initial.Config.BillingAccessController.Bytes())
		//require.Equal(t, config["min_answer"], initial.Config.MinAnswer.BigInt().Bytes())
		//require.Equal(t, config["max_answer"], initial.Config.MaxAnswer.BigInt().Bytes())
		//require.Equal(t, config["decimals"], int32(initial.Config.Decimals))
		//require.Equal(t, config["description"], initial.Config.Description[:])
		//require.Equal(t, config["f"], int32(initial.Config.F))
		//require.Equal(t, config["n"], int32(initial.Config.N))
		//require.Equal(t, config["config_count"], int32(initial.Config.ConfigCount))
		//require.Equal(t, config["latest_config_digest"], initial.Config.LatestConfigDigest[:])
		//require.Equal(t, config["latest_config_block_number"], int64(initial.Config.LatestConfigBlockNumber))
		//require.Equal(t, config["latest_aggregator_round_id"], int32(initial.Config.LatestAggregatorRoundID))
		//require.Equal(t, config["epoch"], int32(initial.Config.Epoch))
		//require.Equal(t, config["round"], int32(initial.Config.Round))
		//require.Equal(t, config["validator"], initial.Config.Validator.Bytes())
		//require.Equal(t, config["flagging_threshold"], int32(initial.Config.FlaggingThreshold))

		//billing, ok := config["billing"].(map[string]interface{})
		//require.True(t, ok, "billing is a map")
		//require.Equal(t, billing["observation_payment"], int32(initial.Config.Billing.ObservationPayment))

		//oracles, ok := state["oracles"].([]interface{})
		//require.True(t, ok, "oracles is an array")
		//require.Equal(t, len(oracles), len(initial.Oracles))
		//for i := 0; i < len(oracles); i++ {
		//	oracle, ok := oracles[i].(map[string]interface{})
		//	require.True(t, ok, "oracle is a map")
		//	require.Equal(t, oracle["transmitter"], initial.Oracles[i].Transmitter.Bytes())
		//	require.Equal(t, oracle["payee"], initial.Oracles[i].Payee.Bytes())
		//	require.Equal(t, oracle["proposed_payee"], initial.Oracles[i].ProposedPayee.Bytes())
		//	require.Equal(t, oracle["payment"], int64(initial.Oracles[i].Payment))
		//	require.Equal(t, oracle["from_round_id"], int32(initial.Oracles[i].FromRoundID))

		//	signer, ok := oracle["signer"].(map[string]interface{})
		//	require.True(t, ok, "signer needs to be a map")
		//	require.Equal(t, signer["key"], initial.Oracles[i].Signer.Key[:])
		//}

		//leftovers, ok := state["leftover_payment"].([]interface{})
		//require.True(t, ok, "leftover_payment is an array")
		//for i := 0; i < len(leftovers); i++ {
		//	leftover, ok := leftovers[i].(map[string]interface{})
		//	require.True(t, ok, "leftover_payment is a map")
		//	require.Equal(t, leftover["payee"], initial.LeftoverPayment[i].Payee.Bytes())
		//	require.Equal(t, leftover["amount"], int64(initial.LeftoverPayment[i].Amount))
		//}
	})

	/*
		t.Run("Answer", func(t *testing.T) {
			initial := generateTransmission(100)
			output := []byte{}
			mapping, err := TranslateToMap(initial)
			require.NoError(t, err)
			serialized, err := transmissionCodec.BinaryFromNative(output, mapping)
			require.NoError(t, err)
			deserialized, _, err := transmissionCodec.NativeFromBinary(serialized)
			require.NoError(t, err)
			transmission, ok := deserialized.(map[string]interface{})
			require.True(t, ok, "should be a map")
			require.Equal(t, transmission["answer"], initial.Answer.Answer.Bytes())
			require.Equal(t, transmission["timestamp"].(int64), int64(initial.Answer.Timestamp))
		})
	*/
}
