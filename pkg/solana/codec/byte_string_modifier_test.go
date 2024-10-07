package codec_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestSolanaAddressModifier(t *testing.T) {
	modifier := codec.SolanaAddressModifier{}

	// Valid Solana address (32 bytes, Base58 encoded)
	validAddressStr := "9nQhQ7iCyY5SgAX2Zm4DtxNh9Ubc4vbiLkiYbX43SDXY"
	validAddressBytes := solana.MustPublicKeyFromBase58(validAddressStr).Bytes()

	// Invalid Solana addresses
	invalidLengthAddressStr := "abc123"

	t.Run("EncodeAddress encodes valid Solana address bytes", func(t *testing.T) {
		encoded, err := modifier.EncodeAddress(validAddressBytes)
		require.NoError(t, err)
		assert.Equal(t, validAddressStr, encoded)
	})

	t.Run("EncodeAddress returns error for invalid byte length", func(t *testing.T) {
		invalidBytes := []byte(invalidLengthAddressStr)
		_, err := modifier.EncodeAddress(invalidBytes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), commontypes.ErrInvalidType.Error())
	})

	t.Run("DecodeAddress decodes valid Solana address", func(t *testing.T) {
		decodedBytes, err := modifier.DecodeAddress(validAddressStr)
		require.NoError(t, err)
		assert.Equal(t, validAddressBytes, decodedBytes)
	})

	t.Run("DecodeAddress returns error for invalid address length", func(t *testing.T) {
		_, err := modifier.DecodeAddress(invalidLengthAddressStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), commontypes.ErrInvalidType.Error())
	})

	t.Run("DecodeAddress returns error for zero-value address", func(t *testing.T) {
		_, err := modifier.DecodeAddress(solana.PublicKey{}.String())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), commontypes.ErrInvalidType.Error())
	})

	t.Run("Length returns 32 for Solana addresses", func(t *testing.T) {
		assert.Equal(t, 32, modifier.Length())
	})
}
