package commoncodec_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

func TestSolanaAddressModifier(t *testing.T) {
	modifier := solcommoncodec.SolanaAddressModifier{}

	// Valid Solana address (32 bytes, Base58 encoded)
	validAddressStr := "9nQhQ7iCyY5SgAX2Zm4DtxNh9Ubc4vbiLkiYbX43SDXY"
	addressNotOnCurveStr := "8opHzTAnfzRpPEx21XtnrVTX28YQuCpAjcn1PczScKh"
	validAddressBytes := solana.MustPublicKeyFromBase58(validAddressStr).Bytes()
	addressNotOnCurveBytes := solana.MustPublicKeyFromBase58(addressNotOnCurveStr).Bytes()
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

	t.Run("DecodeAddress decodes zero-value address", func(t *testing.T) {
		decodedBytes, err := modifier.DecodeAddress(solana.PublicKey{}.String())
		require.NoError(t, err)
		assert.Equal(t, solana.PublicKey{}.Bytes(), decodedBytes)
	})

	t.Run("DecodeAddress returns error for address under 32 chars", func(t *testing.T) {
		// < than 32 chars
		_, err := modifier.DecodeAddress("1111111111111111111111111111111")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), commontypes.ErrInvalidType.Error())
	})

	t.Run("DecodeAddress decodes address not on ed25519 curve", func(t *testing.T) {
		decodedBytes, err := modifier.DecodeAddress(addressNotOnCurveStr)

		require.NoError(t, err)
		assert.Equal(t, addressNotOnCurveBytes, decodedBytes)
	})

	t.Run("Length returns 32 for Solana addresses", func(t *testing.T) {
		assert.Equal(t, solana.PublicKeyLength, modifier.Length())
	})
}
