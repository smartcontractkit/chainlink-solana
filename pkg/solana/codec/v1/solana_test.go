package codecv1_test

import (
	"encoding/json"
	"testing"
	"time"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1/testutils"
)

func TestNewIDLAccountCodec(t *testing.T) {
	/// TODO BCI-3155 this should run the codec interface tests
	t.Parallel()

	ctx := t.Context()
	_, _, entry := newTestIDLAndCodec(t, accountIDLType)

	expected := testutils.DefaultTestStruct
	bts, err := entry.Encode(ctx, expected, testutils.TestStructWithNestedStruct)

	// length of fields + discriminator
	require.Equal(t, 263, len(bts))
	require.NoError(t, err)

	var decoded testutils.StructWithNestedStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, testutils.TestStructWithNestedStruct))
	require.Equal(t, expected, decoded)
}

func TestCodecProperties(t *testing.T) {
	t.Parallel()
	t.Log("newTestIDLAndCodec does not handle eventIDLType and it looks like there is an attempt to deprecate the methods")
	t.Skip()

	tester := &codecInterfaceTester{}
	ctx := t.Context()
	_, _, entry := newTestIDLAndCodec(t, eventIDLType)
	t.Log(entry)

	expected := interfacetests.CreateTestStruct(1, tester)
	bts, err := entry.Encode(ctx, expected, interfacetests.TestItemType)

	// length of fields + discriminator
	require.Equal(t, 262, len(bts))
	require.NoError(t, err)

	var decoded interfacetests.TestStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, interfacetests.TestItemType))
	require.Equal(t, expected, decoded)
}

func TestNewIDLDefinedTypesCodecCodec(t *testing.T) {
	/// TODO BCI-3155 this should run the codec interface tests
	t.Parallel()

	ctx := t.Context()
	_, _, entry := newTestIDLAndCodec(t, definedTypesIDLType)

	expected := testutils.DefaultTestStruct
	bts, err := entry.Encode(ctx, expected, testutils.TestStructWithNestedStructType)

	// length of fields without a discriminator
	require.Equal(t, 255, len(bts))

	require.NoError(t, err)

	var decoded testutils.StructWithNestedStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, testutils.TestStructWithNestedStructType))
	require.Equal(t, expected, decoded)
}

func TestNewIDLCodec_WithModifiers(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	_, _, idlCodec := newTestIDLAndCodec(t, accountIDLType)
	modConfig := codeccommon.ModifiersConfig{
		&codeccommon.RenameModifierConfig{Fields: map[string]string{"Value": "V"}},
	}

	renameMod, err := modConfig.ToModifier(solcommoncodec.DecoderHooks...)
	require.NoError(t, err)

	idlCodecWithMods, err := codecv1.NewNamedModifierCodec(idlCodec, testutils.TestStructWithNestedStruct, renameMod)
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

	var idl codecv1.IDL
	if err := json.Unmarshal([]byte(testutils.CircularDepIDL), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	_, err := codecv1.NewIDLAccountCodec(idl, binary.LittleEndian())

	assert.ErrorIs(t, err, types.ErrInvalidConfig)
}

type idlType string

const (
	accountIDLType      idlType = "account"
	definedTypesIDLType idlType = "types"
	instructionIDLType  idlType = "instruction"
	eventIDLType        idlType = "event"
)

func newTestIDLAndCodec(t *testing.T, idlTP idlType) (string, codecv1.IDL, types.RemoteCodec) {
	t.Helper()

	var idlDef string

	//nolint:exhaustive
	switch idlTP {
	case accountIDLType, definedTypesIDLType:
		idlDef = testutils.JSONIDLWithAllTypes
	case eventIDLType:
		defs := testutils.CodecDefs[testutils.TestEventItem]
		idlDef = defs.IDL
	}

	var idl codecv1.IDL
	if err := json.Unmarshal([]byte(idlDef), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	var (
		entry types.RemoteCodec
		err   error
	)

	//nolint:exhaustive
	switch idlTP {
	case accountIDLType:
		entry, err = codecv1.NewIDLAccountCodec(idl, binary.LittleEndian())
	case definedTypesIDLType:
		entry, err = codecv1.NewIDLDefinedTypesCodec(idl, binary.LittleEndian())
	}

	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	require.NotNil(t, entry, "test codec should not be nil")

	return idlDef, idl, entry
}
