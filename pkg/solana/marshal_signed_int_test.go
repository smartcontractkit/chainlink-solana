package solana

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalSignedInt(t *testing.T) {
	var tt = []struct {
		bytesVal  string
		size      uint
		expected  *big.Int
		expectErr bool
	}{
		{
			"ffffffffffffffff",
			8,
			big.NewInt(-1),
			false,
		},
		{
			"fffffffffffffffe",
			8,
			big.NewInt(-2),
			false,
		},
		{
			"0000000000000000",
			8,
			big.NewInt(0),
			false,
		},
		{
			"0000000000000001",
			8,
			big.NewInt(1),
			false,
		},
		{
			"0000000000000002",
			8,
			big.NewInt(2),
			false,
		},
		{
			"7fffffffffffffff",
			8,
			big.NewInt(9223372036854775807), // 2^63 - 1
			false,
		},
		{
			"00000000000000000000000000000000",
			16,
			big.NewInt(0),
			false,
		},
		{
			"00000000000000000000000000000001",
			16,
			big.NewInt(1),
			false,
		},
		{
			"00000000000000000000000000000002",
			16,
			big.NewInt(2),
			false,
		},
		{
			"7fffffffffffffffffffffffffffffff", // 2^127 - 1
			16,
			big.NewInt(0).Sub(big.NewInt(0).Lsh(big.NewInt(1), 127), big.NewInt(1)),
			false,
		},
		{
			"ffffffffffffffffffffffffffffffff",
			16,
			big.NewInt(-1),
			false,
		},
		{
			"fffffffffffffffffffffffffffffffe",
			16,
			big.NewInt(-2),
			false,
		},
		{
			"000000000000000000000000000000000000000000000000",
			24,
			big.NewInt(0),
			false,
		},
		{
			"000000000000000000000000000000000000000000000001",
			24,
			big.NewInt(1),
			false,
		},
		{
			"000000000000000000000000000000000000000000000002",
			24,
			big.NewInt(2),
			false,
		},
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffff",
			24,
			big.NewInt(-1),
			false,
		},
		{
			"fffffffffffffffffffffffffffffffffffffffffffffffe",
			24,
			big.NewInt(-2),
			false,
		},
	}
	for _, tc := range tt {
		tc := tc
		b, err := hex.DecodeString(tc.bytesVal)
		require.NoError(t, err)
		i, err := ToBigInt(b, tc.size)
		require.NoError(t, err)
		assert.Equal(t, i.String(), tc.expected.String())

		// Marshalling back should give us the same bytes
		bAfter, err := ToBytes(i, tc.size)
		require.NoError(t, err)
		assert.Equal(t, tc.bytesVal, hex.EncodeToString(bAfter))
	}

	var tt2 = []struct {
		o         *big.Int
		numBytes  uint
		expectErr bool
	}{
		{
			big.NewInt(128),
			1,
			true,
		},
		{
			big.NewInt(-129),
			1,
			true,
		},
		{
			big.NewInt(-128),
			1,
			false,
		},
		{
			big.NewInt(2147483648),
			4,
			true,
		},
		{
			big.NewInt(2147483647),
			4,
			false,
		},
		{
			big.NewInt(-2147483649),
			4,
			true,
		},
		{
			big.NewInt(-2147483648),
			4,
			false,
		},
	}
	for _, tc := range tt2 {
		tc := tc
		_, err := ToBytes(tc.o, tc.numBytes)
		if tc.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}
