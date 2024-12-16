package codec

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type testErrEncodeEntry struct {
	entry
	codecType commonencodings.TypeCodec
}

func (t *testErrEncodeEntry) Encode(_ interface{}, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("encode error")
}

func (t *testErrEncodeEntry) GetType() reflect.Type {
	return commonencodings.Empty{}.GetType()
}

type testErrEncodeTypeEntry struct {
	entry
	tCodec commonencodings.TypeCodec
}

func (e *testErrEncodeTypeEntry) GetCodecType() commonencodings.TypeCodec {
	return e.tCodec
}

func TestEncoder_Encode_Errors(t *testing.T) {
	someType := "some-type"

	t.Run("error when item type not found", func(t *testing.T) {
		e := &Encoder{definitions: map[string]Entry{}}
		_, err := e.Encode(tests.Context(t), nil, "non-existent-type")
		require.Error(t, err)
		require.ErrorIs(t, err, commontypes.ErrInvalidType)
		require.Contains(t, err.Error(), "cannot find type non-existent-type")
	})

	t.Run("error when convert fails because of unexpected type", func(t *testing.T) {
		e := &Encoder{
			definitions: map[string]Entry{
				someType: &testErrEncodeEntry{},
			},
		}
		_, err := e.Encode(tests.Context(t), nil, someType)
		require.Error(t, err)
	})

	t.Run("error when entry encode fails", func(t *testing.T) {
		e := &Encoder{
			definitions: map[string]Entry{
				someType: &testErrEncodeEntry{codecType: commonencodings.Empty{}},
			},
		}
		_, err := e.Encode(tests.Context(t), make(map[string]interface{}), someType)
		require.ErrorContains(t, err, "encode error")
	})
}

type testErrGetSize struct {
	commonencodings.Empty
	retType reflect.Type
}

func (t testErrGetSize) GetType() reflect.Type {
	return t.retType
}

func (t testErrGetSize) Size(_ int) (int, error) {
	return 0, fmt.Errorf("size error")
}

func TestEncoder_GetMaxEncodingSize_Errors(t *testing.T) {
	t.Run("error when entry for item type is missing", func(t *testing.T) {
		e := &Encoder{definitions: map[string]Entry{}}
		_, err := e.GetMaxEncodingSize(tests.Context(t), 10, "no-entry-type")
		require.Error(t, err)
		require.ErrorIs(t, err, commontypes.ErrInvalidType)
		require.Contains(t, err.Error(), "nil entry")
	})

	t.Run("error when size calculation fails", func(t *testing.T) {
		someType := "some-type"
		e := &Encoder{
			definitions: map[string]Entry{
				someType: &testErrEncodeTypeEntry{tCodec: testErrGetSize{}},
			},
		}

		_, err := e.GetMaxEncodingSize(tests.Context(t), 0, someType)
		require.Error(t, err)
		require.Contains(t, err.Error(), "size error")
	})
}
