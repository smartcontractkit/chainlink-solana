package monitoring

import (
	"testing"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/stretchr/testify/require"
)

func TestSchemas(t *testing.T) {
	solanaConfig := config.Solana{}
	feedConfig := Feed{}
	transmissionEnvelope := TransmissionEnvelope{}
	configEnvelope := ConfigEnvelope{}
	t.Run("encode an empty configSetSimplified message", func(t *testing.T) {
		mapping, err := MakeConfigSetSimplifiedMapping(configEnvelope, feedConfig)
		require.NoError(t, err)
		_, err = configSetSimplifiedCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
	t.Run("encode an empty transmission message", func(t *testing.T) {
		mapping, err := MakeTransmissionMapping(transmissionEnvelope, solanaConfig, feedConfig)
		require.NoError(t, err)
		_, err = transmissionCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
}
