package codec_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

type testErrDecodeEntry struct {
	codec.CodecEntry
}

func (t *testErrDecodeEntry) Decode(_ []byte) (interface{}, []byte, error) {
	return nil, nil, fmt.Errorf("decode error")
}

type testErrDecodeRemainingBytes struct {
	codec.CodecEntry
}

func (t *testErrDecodeRemainingBytes) Decode(_ []byte) (interface{}, []byte, error) {
	return nil, []byte{1}, nil
}

func TestDecoder_Decode_Errors(t *testing.T) {
	var into interface{}
	someType := "some-type"
	t.Run("error when item type not found", func(t *testing.T) {
		d := &codec.Decoder{Definitions: map[string]codec.Entry{}}
		d.Definitions[someType] = &codec.CodecEntry{}

		nonExistentType := "non-existent"
		err := d.Decode(tests.Context(t), []byte{}, &into, nonExistentType)
		require.ErrorIs(t, err, fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, nonExistentType))
	})

	t.Run("error when underlying entry decode fails", func(t *testing.T) {
		d := &codec.Decoder{Definitions: map[string]codec.Entry{}}
		d.Definitions[someType] = &testErrDecodeEntry{}
		require.Error(t, d.Decode(tests.Context(t), []byte{}, &into, someType))
	})

	t.Run("error when remaining bytes exist after decode", func(t *testing.T) {
		d := &codec.Decoder{Definitions: map[string]codec.Entry{}}
		d.Definitions[someType] = &testErrDecodeRemainingBytes{}
		require.Error(t, d.Decode(tests.Context(t), []byte{}, &into, someType))
	})
}

type testErrGetMaxDecodingSize struct {
	codec.CodecEntry
}

type testErrGetMaxDecodingSizeCodecType struct {
	commonencodings.Empty
}

func (t testErrGetMaxDecodingSizeCodecType) Size(_ int) (int, error) {
	return 0, fmt.Errorf("error")
}

func (t *testErrGetMaxDecodingSize) GetCodecType() commonencodings.TypeCodec {
	return testErrGetMaxDecodingSizeCodecType{}
}

func TestDecoder_GetMaxDecodingSize_Errors(t *testing.T) {
	someType := "some-type"

	t.Run("error when entry for item type is missing", func(t *testing.T) {
		d := &codec.Decoder{Definitions: map[string]codec.Entry{}}
		d.Definitions[someType] = &codec.CodecEntry{}

		nonExistentType := "non-existent"
		_, err := d.GetMaxDecodingSize(tests.Context(t), 0, nonExistentType)
		require.ErrorIs(t, err, fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, nonExistentType))
	})

	t.Run("error when underlying entry decode fails", func(t *testing.T) {
		d := &codec.Decoder{Definitions: map[string]codec.Entry{}}
		d.Definitions[someType] = &testErrGetMaxDecodingSize{}

		_, err := d.GetMaxDecodingSize(tests.Context(t), 0, someType)
		require.Error(t, err)
	})
}
