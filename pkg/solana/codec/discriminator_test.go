package codec_test

import (
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestDiscriminator(t *testing.T) {
	t.Run("encode and decode return the discriminator", func(t *testing.T) {
		tmp := sha256.Sum256([]byte("account:Foo"))
		expected := tmp[:8]
		c := codec.NewDiscriminator("Foo")
		encoded, err := c.Encode(&expected, nil)
		require.NoError(t, err)
		require.Equal(t, expected, encoded)
		actual, remaining, err := c.Decode(encoded)
		require.NoError(t, err)
		require.Equal(t, &expected, actual)
		require.Len(t, remaining, 0)
	})

	t.Run("encode returns an error if the discriminator is invalid", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		_, err := c.Encode(&[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, nil)
		require.True(t, errors.Is(err, types.ErrInvalidType))
	})

	t.Run("encode injects the discriminator if it's not provided", func(t *testing.T) {
		tmp := sha256.Sum256([]byte("account:Foo"))
		expected := tmp[:8]
		c := codec.NewDiscriminator("Foo")
		encoded, err := c.Encode(nil, nil)
		require.NoError(t, err)
		require.Equal(t, expected, encoded)
		encoded, err = c.Encode((*[]byte)(nil), nil)
		require.NoError(t, err)
		require.Equal(t, expected, encoded)
	})

	t.Run("decode returns an error if the encoded value is too short", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		_, _, err := c.Decode([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
		require.True(t, errors.Is(err, types.ErrInvalidEncoding))
	})

	t.Run("decode returns an error if the discriminator is invalid", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		_, _, err := c.Decode([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
		require.True(t, errors.Is(err, types.ErrInvalidEncoding))
	})

	t.Run("encode returns an error if the value is not a byte slice", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		_, err := c.Encode(42, nil)
		require.True(t, errors.Is(err, types.ErrInvalidType))
	})

	t.Run("GetType returns the type of the discriminator", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		require.Equal(t, reflect.TypeOf(&[]byte{}), c.GetType())
	})

	t.Run("Size returns the length of the discriminator", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		size, err := c.Size(0)
		require.NoError(t, err)
		require.Equal(t, 8, size)
	})

	t.Run("FixedSize returns the length of the discriminator", func(t *testing.T) {
		c := codec.NewDiscriminator("Foo")
		size, err := c.FixedSize()
		require.NoError(t, err)
		require.Equal(t, 8, size)
	})
}
