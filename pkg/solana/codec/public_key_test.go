package codec_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestPublicKey(t *testing.T) {
	t.Run("encode and decode solana public key from []byte", func(t *testing.T) {
		codec := codec.NewPublicKey()
		into := []byte{}
		encoded, err := codec.Encode([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, into)
		require.NoError(t, err)
		require.Len(t, encoded, solana.PublicKeyLength)
		decoded, remaining, err := codec.Decode(encoded)
		require.NoError(t, err)
		require.Len(t, remaining, 0)
		require.Len(t, decoded, solana.PublicKeyLength)
		require.IsType(t, solana.PublicKey{}, decoded)
	})

	t.Run("encode and decode solana public key from *[]byte", func(t *testing.T) {
		codec := codec.NewPublicKey()
		pubKeyBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		into := []byte{}
		encoded, err := codec.Encode(&pubKeyBytes, into)
		require.NoError(t, err)
		require.Len(t, encoded, solana.PublicKeyLength)
		decoded, remaining, err := codec.Decode(encoded)
		require.NoError(t, err)
		require.Len(t, remaining, 0)
		require.Len(t, decoded, solana.PublicKeyLength)
		require.IsType(t, solana.PublicKey{}, decoded)
	})

	t.Run("encode and decode solana public key from solana public key type", func(t *testing.T) {
		codec := codec.NewPublicKey()
		into := []byte{}
		encoded, err := codec.Encode(solana.PublicKey{}, into)
		require.NoError(t, err)
		require.Len(t, encoded, solana.PublicKeyLength)
		decoded, remaining, err := codec.Decode(encoded)
		require.NoError(t, err)
		require.Len(t, remaining, 0)
		require.Len(t, decoded, solana.PublicKeyLength)
		require.IsType(t, solana.PublicKey{}, decoded)
	})

	t.Run("encode and decode solana public key from string", func(t *testing.T) {
		codec := codec.NewPublicKey()
		into := []byte{}
		encoded, err := codec.Encode("11111111111111111111111111111111", into)
		require.NoError(t, err)
		require.Len(t, encoded, solana.PublicKeyLength)
		decoded, remaining, err := codec.Decode(encoded)
		require.NoError(t, err)
		require.Len(t, remaining, 0)
		require.Len(t, decoded, solana.PublicKeyLength)
		require.IsType(t, solana.PublicKey{}, decoded)
	})

	t.Run("error encoding if invalid solana public key provided", func(t *testing.T) {
		codec := codec.NewPublicKey()
		into := []byte{}
		_, err := codec.Encode("1", into)
		require.Error(t, err)
	})
}
