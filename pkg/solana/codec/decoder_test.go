package codec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type testErrDecodeEntry struct {
	entry
}

func (t *testErrDecodeEntry) Decode(_ []byte) (interface{}, []byte, error) {
	return nil, nil, fmt.Errorf("decode error")
}

type testErrDecodeRemainingBytes struct {
	entry
}

func (t *testErrDecodeRemainingBytes) Decode(_ []byte) (interface{}, []byte, error) {
	return struct{}{}, []byte{1}, nil
}

func TestDecoder_Decode_Errors(t *testing.T) {
	var into interface{}
	someType := "some-type"
	t.Run("error when item type not found", func(t *testing.T) {
		d := &Decoder{definitions: map[string]Entry{}}
		d.definitions[someType] = &entry{}

		nonExistentType := "non-existent"
		err := d.Decode(tests.Context(t), []byte{}, &into, nonExistentType)
		require.ErrorIs(t, err, fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, nonExistentType))
	})

	t.Run("error when underlying entry decode fails", func(t *testing.T) {
		d := &Decoder{definitions: map[string]Entry{}}
		d.definitions[someType] = &testErrDecodeEntry{}
		require.Error(t, d.Decode(tests.Context(t), []byte{}, &into, someType))
	})

	t.Run("remaining bytes exist after decode is ok", func(t *testing.T) {
		d := &Decoder{definitions: map[string]Entry{}}
		d.definitions[someType] = &testErrDecodeRemainingBytes{}
		require.NoError(t, d.Decode(tests.Context(t), []byte{}, &into, someType))
	})
}

type testErrGetMaxDecodingSize struct {
	entry
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
		d := &Decoder{definitions: map[string]Entry{}}
		d.definitions[someType] = &entry{}

		nonExistentType := "non-existent"
		_, err := d.GetMaxDecodingSize(tests.Context(t), 0, nonExistentType)
		require.ErrorIs(t, err, fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, nonExistentType))
	})

	t.Run("error when underlying entry decode fails", func(t *testing.T) {
		d := &Decoder{definitions: map[string]Entry{}}
		d.definitions[someType] = &testErrGetMaxDecodingSize{}

		_, err := d.GetMaxDecodingSize(tests.Context(t), 0, someType)
		require.Error(t, err)
	})
}
