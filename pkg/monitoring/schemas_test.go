package monitoring

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemas(t *testing.T) {
	chainCfg := SolanaConfig{}
	feedConfig := generateFeedConfig()
	envelope := Envelope{}
	t.Run("encode an empty configSetSimplified message", func(t *testing.T) {
		mapping, err := MakeConfigSetSimplifiedMapping(envelope, feedConfig)
		require.NoError(t, err)
		_, err = configSetSimplifiedCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
	t.Run("encode an empty transmission message", func(t *testing.T) {
		mapping, err := MakeTransmissionMapping(envelope, chainCfg, feedConfig)
		require.NoError(t, err)
		_, err = transmissionCodec.BinaryFromNative(nil, mapping)
		require.NoError(t, err)
	})
}
