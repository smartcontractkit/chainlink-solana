package codecv1_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

func TestOnRampAddressRoundTrip(t *testing.T) {
	t.Parallel()

	codec := codecv1.NewOnRampAddress(binary.LittleEndian())

	addr := make([]byte, 32)
	for i := range addr {
		addr[i] = byte(i)
	}

	encoded, err := codec.Encode(addr, nil)
	require.NoError(t, err)
	require.Len(t, encoded, 68)

	decoded, remaining, err := codec.Decode(encoded)
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, addr, decoded)
}

func TestOnRampAddressDecodeErrors(t *testing.T) {
	t.Parallel()

	codec := codecv1.NewOnRampAddress(binary.LittleEndian())

	t.Run("short buffer", func(t *testing.T) {
		t.Parallel()

		_, _, err := codec.Decode(make([]byte, 63))
		require.ErrorIs(t, err, types.ErrInvalidEncoding)
	})

	t.Run("length exceeds 64", func(t *testing.T) {
		t.Parallel()

		encoded, err := codec.Encode(make([]byte, 64), nil)
		require.NoError(t, err)

		// corrupt the trailing uint32 length to 65
		encoded[64] = 65

		_, _, err = codec.Decode(encoded)
		require.ErrorIs(t, err, types.ErrInvalidEncoding)
	})
}
