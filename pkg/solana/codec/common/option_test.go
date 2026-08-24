package commoncodec

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
)

type testOptionInnerCodec struct{}

var _ encodings.TypeCodec = testOptionInnerCodec{}

func (testOptionInnerCodec) Encode(value any, into []byte) ([]byte, error) {
	return into, nil
}

func (testOptionInnerCodec) Decode(encoded []byte) (any, []byte, error) {
	return int32(42), encoded, nil
}

func (testOptionInnerCodec) GetType() reflect.Type {
	return reflect.TypeOf(int32(0))
}

func (testOptionInnerCodec) Size(_ int) (int, error) {
	return 0, nil
}

func (testOptionInnerCodec) FixedSize() (int, error) {
	return 0, nil
}

type panicOptionInnerCodec struct{}

var _ encodings.TypeCodec = panicOptionInnerCodec{}

func (panicOptionInnerCodec) Encode(value any, into []byte) ([]byte, error) {
	return into, nil
}

func (panicOptionInnerCodec) Decode(encoded []byte) (any, []byte, error) {
	panic("inner Decode should not be called")
}

func (panicOptionInnerCodec) GetType() reflect.Type {
	return reflect.TypeOf(int32(0))
}

func (panicOptionInnerCodec) Size(_ int) (int, error) {
	return 0, nil
}

func (panicOptionInnerCodec) FixedSize() (int, error) {
	return 0, nil
}

func TestOptionDecode_EmptyInputReturnsError(t *testing.T) {
	codec := NewOption(panicOptionInnerCodec{})

	decoded, remaining, err := codec.Decode(nil)
	require.Error(t, err)
	require.EqualError(t, err, "cannot decode option: empty input")
	require.Nil(t, decoded)
	require.Nil(t, remaining)
}

func TestOptionDecode_PrefixOneDelegatesToInnerCodec(t *testing.T) {
	codec := NewOption(testOptionInnerCodec{})

	decoded, remaining, err := codec.Decode([]byte{1, 0xaa, 0xbb})
	require.NoError(t, err)
	require.Equal(t, int32(42), decoded)
	require.Equal(t, []byte{0xaa, 0xbb}, remaining)
}

func TestOptionDecode_InvalidPrefixReturnsError(t *testing.T) {
	codec := NewOption(testOptionInnerCodec{})

	decoded, remaining, err := codec.Decode([]byte{2})
	require.Error(t, err)
	require.EqualError(t, err, "expected either 0 or 1, got 2")
	require.Nil(t, decoded)
	require.Equal(t, []byte{2}, remaining)
}
