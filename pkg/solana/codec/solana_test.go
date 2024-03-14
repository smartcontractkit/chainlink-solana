package codec_test

import (
	"encoding/json"
	"testing"
	"time"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
)

func TestNewIDLCodec(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	_, _, entry := newTestIDLAndCodec(t)

	expected := testutils.DefaultTestStruct
	bts, err := entry.Encode(ctx, expected, testutils.TestStructWithNestedStruct)

	require.NoError(t, err)

	var decoded testutils.StructWithNestedStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, testutils.TestStructWithNestedStruct))
	require.Equal(t, expected, decoded)
}

func TestNewIDLCodec_WithModifiers(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	_, _, idlCodec := newTestIDLAndCodec(t)
	modConfig := codeccommon.ModifiersConfig{
		&codeccommon.RenameModifierConfig{Fields: map[string]string{"Value": "V"}},
	}

	renameMod, err := modConfig.ToModifier(codec.DecoderHooks...)
	require.NoError(t, err)

	idlCodecWithMods, err := codec.NewNamedModifierCodec(idlCodec, testutils.TestStructWithNestedStruct, renameMod)
	require.NoError(t, err)

	type modifiedTestStruct struct {
		V                uint8
		InnerStruct      testutils.ObjectRef1
		BasicNestedArray [][]uint32
		Option           *string
		DefinedArray     []testutils.ObjectRef2
		BasicVector      []string
		TimeVal          int64
		DurationVal      time.Duration
		PublicKey        ag_solana.PublicKey
		EnumVal          uint8
	}

	expected := modifiedTestStruct{
		V:                testutils.DefaultTestStruct.Value,
		InnerStruct:      testutils.DefaultTestStruct.InnerStruct,
		BasicNestedArray: testutils.DefaultTestStruct.BasicNestedArray,
		Option:           testutils.DefaultTestStruct.Option,
		DefinedArray:     testutils.DefaultTestStruct.DefinedArray,
		BasicVector:      testutils.DefaultTestStruct.BasicVector,
		TimeVal:          testutils.DefaultTestStruct.TimeVal,
		DurationVal:      testutils.DefaultTestStruct.DurationVal,
		PublicKey:        testutils.DefaultTestStruct.PublicKey,
		EnumVal:          testutils.DefaultTestStruct.EnumVal,
	}

	withModsBts, err := idlCodecWithMods.Encode(ctx, expected, testutils.TestStructWithNestedStruct)
	require.NoError(t, err)

	noModsBts, err := idlCodec.Encode(ctx, testutils.DefaultTestStruct, testutils.TestStructWithNestedStruct)

	// the codec without modifiers should encode an unmodified struct to the same bytes
	// as the codec with modifiers encodes a modified struct
	require.NoError(t, err)
	require.Equal(t, withModsBts, noModsBts)

	var decoded modifiedTestStruct

	// the codec with modifiers should decode from unmodified bytes into a modified struct
	require.NoError(t, idlCodecWithMods.Decode(ctx, noModsBts, &decoded, testutils.TestStructWithNestedStruct))
	require.Equal(t, expected, decoded)

	var unmodifiedDecoded testutils.StructWithNestedStruct

	// the codec without modifiers should decode from unmodified bytes to the same values as
	// modified struct
	require.NoError(t, idlCodec.Decode(ctx, noModsBts, &unmodifiedDecoded, testutils.TestStructWithNestedStruct))
	require.Equal(t, expected.V, unmodifiedDecoded.Value)
	require.Equal(t, expected.TimeVal, unmodifiedDecoded.TimeVal)
	require.Equal(t, expected.DurationVal, unmodifiedDecoded.DurationVal)
	require.Equal(t, expected.PublicKey, unmodifiedDecoded.PublicKey)
	require.Equal(t, expected.EnumVal, unmodifiedDecoded.EnumVal)
}

func TestNewIDLCodec_CircularDependency(t *testing.T) {
	t.Parallel()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(testutils.CircularDepIDL), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	_, err := codec.NewIDLCodec(idl)

	assert.ErrorIs(t, err, types.ErrInvalidConfig)
}

func newTestIDLAndCodec(t *testing.T) (string, codec.IDL, encodings.CodecFromTypeCodec) {
	t.Helper()

	var idl codec.IDL
	if err := json.Unmarshal([]byte(testutils.JSONIDLWithAllTypes), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	entry, err := codec.NewIDLCodec(idl)
	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	require.NotNil(t, entry)

	return testutils.JSONIDLWithAllTypes, idl, entry
}
