package monitoring

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapping(t *testing.T) {
	t.Run("MakeSimplifiedConfigSetMapping", func(t *testing.T) {
		config, _, offchainConfig, _, err := generateContractConfig(31)
		require.NoError(t, err)
		envelope := ConfigEnvelope{config}

		feedConfig := generateFeedConfig()

		mapping, err := MakeConfigSetSimplifiedMapping(envelope, feedConfig)
		require.NoError(t, err)
		var output []byte
		serialized, err := configSetSimplifiedCodec.BinaryFromNative(output, mapping)
		require.NoError(t, err)
		deserialized, _, err := configSetSimplifiedCodec.NativeFromBinary(serialized)
		require.NoError(t, err)

		configSetSimplified, ok := deserialized.(map[string]interface{})
		require.True(t, ok)

		oracles, err := createConfigSetSimplifiedOracles(offchainConfig.OffchainPublicKeys, offchainConfig.PeerIds, config.Transmitters)
		require.NoError(t, err)

		require.Equal(t, configSetSimplified["config_digest"], base64.StdEncoding.EncodeToString(envelope.ContractConfig.ConfigDigest[:]))
		require.Equal(t, configSetSimplified["block_number"], []byte{})
		require.Equal(t, configSetSimplified["delta_progress"], uint64ToBeBytes(offchainConfig.DeltaProgressNanoseconds))
		require.Equal(t, configSetSimplified["delta_resend"], uint64ToBeBytes(offchainConfig.DeltaResendNanoseconds))
		require.Equal(t, configSetSimplified["delta_round"], uint64ToBeBytes(offchainConfig.DeltaRoundNanoseconds))
		require.Equal(t, configSetSimplified["delta_grace"], uint64ToBeBytes(offchainConfig.DeltaGraceNanoseconds))
		require.Equal(t, configSetSimplified["delta_stage"], uint64ToBeBytes(offchainConfig.DeltaStageNanoseconds))
		require.Equal(t, configSetSimplified["r_max"], int64(offchainConfig.RMax))
		require.Equal(t, configSetSimplified["f"], int32(config.F))
		require.Equal(t, configSetSimplified["signers"], jsonMarshalToString(t, config.Signers))
		require.Equal(t, configSetSimplified["transmitters"], jsonMarshalToString(t, config.Transmitters))
		require.Equal(t, configSetSimplified["s"], jsonMarshalToString(t, offchainConfig.S))
		require.Equal(t, configSetSimplified["oracles"], string(oracles))
		require.Equal(t, configSetSimplified["feed_state_account"], feedConfig.GetContractAddress())
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
		require.Equal(t, transmission["block_number"], []byte{})

		answer, ok := transmission["answer"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, answer["data"], initial.LatestAnswer.Bytes())
		require.Equal(t, answer["timestamp"].(int64), initial.LatestTimestamp.Unix())

		solanaChainConfig, ok := transmission["solana_chain_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, solanaChainConfig["network_name"], solanaConfig.NetworkName)
		require.Equal(t, solanaChainConfig["network_id"], solanaConfig.NetworkID)
		require.Equal(t, solanaChainConfig["chain_id"], solanaConfig.ChainID)

		decodedFeedConfig, ok := transmission["feed_config"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, decodedFeedConfig, feedConfig.ToMapping())
	})
}

// Helpers

func jsonMarshalToString(t *testing.T, i interface{}) string {
	s, err := json.Marshal(i)
	require.NoError(t, err)
	return string(s)
}

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
