package codec_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestSolanaCrossChainAmountCodec(t *testing.T) {
	ccaCodec := codec.NewCrossChainAmount()

	t.Run("successfully encodes and decodes cross chain amount", func(t *testing.T) {
		var into []byte
		value := ccipocr3.NewBigInt(big.NewInt(1))
		// encode value
		into, err := ccaCodec.Encode(value, into)
		require.NoError(t, err)
		require.Len(t, into, codec.CrossChainAmountLength)
		expectedBytes := make([]byte, codec.CrossChainAmountLength)
		expectedBytes[0] = 1 // set first digit in LE encoded bytes to 1 for big.Into value of 1
		require.Equal(t, expectedBytes, into)
		// decode value
		decodedVal, remaining, err := ccaCodec.Decode(into)
		require.NoError(t, err)
		require.Len(t, remaining, 0) // expected no extra bytes
		require.Equal(t, value, decodedVal)
	})

	t.Run("successfully encodes the first 32 bytes of encoded slice and returns remaining", func(t *testing.T) {
		encoded := make([]byte, 48)
		encoded[0] = 1 // set first digit in LE encoded bytes to 1 for big.Into value of 1
		decoded, remaining, err := ccaCodec.Decode(encoded)
		require.NoError(t, err)
		require.Len(t, remaining, len(encoded)-codec.CrossChainAmountLength)
		require.Equal(t, ccipocr3.NewBigInt(big.NewInt(1)), decoded)
	})

	t.Run("fails to encode if value is not big.Int or ccip BigInt", func(t *testing.T) {
		var into []byte
		_, err := ccaCodec.Encode(uint64(1), into)
		require.Error(t, err)
	})

	t.Run("fails to decode if not enough bytes provided", func(t *testing.T) {
		badEncoded := make([]byte, 31)
		_, _, err := ccaCodec.Decode(badEncoded)
		require.Error(t, err)
	})
}
