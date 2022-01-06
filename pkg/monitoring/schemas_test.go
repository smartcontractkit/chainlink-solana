package monitoring

import (
	"testing"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/stretchr/testify/require"
)

func TestSchemas(t *testing.T) {
	solanaConfig := config.Solana{}
	feedConfig := config.Feed{}
	transmission := TransmissionEnvelope{}
	state := StateEnvelope{}
	t.Run("encode an empty configSet message", func(t *testing.T) {
		mapping, err := MakeConfigSetMapping(state, solanaConfig, feedConfig)
		require.NoError(t, err)
		_, err = configSetCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
	t.Run("encode an empty configSetSimplified message", func(t *testing.T) {
		mapping, err := MakeConfigSetSimplifiedMapping(state, feedConfig)
		require.NoError(t, err)
		_, err = configSetSimplifiedCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
	t.Run("encode an empty transmission message", func(t *testing.T) {
		mapping, err := MakeTransmissionMapping(transmission, solanaConfig, feedConfig)
		require.NoError(t, err)
		_, err = transmissionCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
}
