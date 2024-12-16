package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
)

func TestNewIDLAccountCodec(t *testing.T) {
	/// TODO BCI-3155 this should run the codec interface tests
	t.Parallel()

	ctx := tests.Context(t)
	_, _, entry := newTestIDLAndCodec(t, true)

	expected := testutils.DefaultTestStruct
	bts, err := entry.Encode(ctx, expected, testutils.TestStructWithNestedStruct)

	// length of fields + discriminator
	require.Equal(t, 262, len(bts))

	require.NoError(t, err)

	var decoded testutils.StructWithNestedStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, testutils.TestStructWithNestedStruct))
	require.Equal(t, expected, decoded)
}

func TestNewIDLDefinedTypesCodecCodec(t *testing.T) {
	/// TODO BCI-3155 this should run the codec interface tests
	t.Parallel()

	ctx := tests.Context(t)
	_, _, entry := newTestIDLAndCodec(t, false)

	expected := testutils.DefaultTestStruct
	bts, err := entry.Encode(ctx, expected, testutils.TestStructWithNestedStructType)

	// length of fields without a discriminator
	require.Equal(t, 254, len(bts))

	require.NoError(t, err)

	var decoded testutils.StructWithNestedStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, testutils.TestStructWithNestedStructType))
	require.Equal(t, expected, decoded)
}

func TestNewIDLCodec_CircularDependency(t *testing.T) {
	t.Parallel()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(testutils.CircularDepIDL), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	_, err := codec.NewIDLAccountCodec(idl, binary.LittleEndian())

	assert.ErrorIs(t, err, types.ErrInvalidConfig)
}

func newTestIDLAndCodec(t *testing.T, account bool) (string, codec.IDL, types.RemoteCodec) {
	t.Helper()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(testutils.JSONIDLWithAllTypes), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	var entry types.RemoteCodec
	var err error
	if account {
		entry, err = codec.NewIDLAccountCodec(idl, binary.LittleEndian())
	} else {
		entry, err = codec.NewIDLDefinedTypesCodec(idl, binary.LittleEndian())
	}

	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	require.NotNil(t, entry)

	return testutils.JSONIDLWithAllTypes, idl, entry
}
