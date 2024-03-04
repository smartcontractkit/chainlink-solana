package codec_test

import (
	"testing"

	"github.com/test-go/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestNewIDLCodec(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	_, _, entry := codec.NewTestIDLAndCodec(t)

	expected := codec.DefaultTestStruct
	bts, err := entry.Encode(ctx, expected, codec.TestStructWithNestedStruct)

	require.NoError(t, err)

	var decoded codec.StructWithNestedStruct

	require.NoError(t, entry.Decode(ctx, bts, &decoded, codec.TestStructWithNestedStruct))
	require.Equal(t, expected, decoded)
}

func TestNewIDLCodec_WithModifiers(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	_, _, idlCodec := codec.NewTestIDLAndCodec(t)
	modConfig := commoncodec.ModifiersConfig{
		&commoncodec.RenameModifierConfig{Fields: map[string]string{"Value": "V"}},
	}

	renameMod, err := modConfig.ToModifier(codec.DecoderHooks...)
	require.NoError(t, err)

	idlCodecWithMods, err := codec.NewNamedModifierCodec(idlCodec, codec.TestStructWithNestedStruct, renameMod)
	require.NoError(t, err)

	// the test is setup using a codec with and without modifiers

	type modifiedTestStruct struct {
		V                uint8
		InnerStruct      codec.ObjectRef1
		BasicNestedArray [][]uint32
		Option           *string
		DefinedArray     []codec.ObjectRef2
	}

	expected := modifiedTestStruct{
		V:                codec.DefaultTestStruct.Value,
		InnerStruct:      codec.DefaultTestStruct.InnerStruct,
		BasicNestedArray: codec.DefaultTestStruct.BasicNestedArray,
		Option:           codec.DefaultTestStruct.Option,
		DefinedArray:     codec.DefaultTestStruct.DefinedArray,
	}

	withModsBts, err := idlCodecWithMods.Encode(ctx, expected, codec.TestStructWithNestedStruct)
	require.NoError(t, err)

	noModsBts, err := idlCodec.Encode(ctx, codec.DefaultTestStruct, codec.TestStructWithNestedStruct)

	// the codec without modifiers should encode an unmodified struct to the same bytes
	// as the codec with modifiers encodes a modified struct
	require.NoError(t, err)
	require.Equal(t, withModsBts, noModsBts)

	var decoded modifiedTestStruct

	// the codec with modifiers should decode from unmodified bytes into a modified struct
	require.NoError(t, idlCodecWithMods.Decode(ctx, noModsBts, &decoded, codec.TestStructWithNestedStruct))
	require.Equal(t, expected, decoded)

	var unmodifiedDecoded codec.StructWithNestedStruct

	// the codec without modifiers should decode from unmodified bytes to the same values as
	// modified struct
	require.NoError(t, idlCodec.Decode(ctx, noModsBts, &unmodifiedDecoded, codec.TestStructWithNestedStruct))
	require.Equal(t, expected.V, unmodifiedDecoded.Value)
}
