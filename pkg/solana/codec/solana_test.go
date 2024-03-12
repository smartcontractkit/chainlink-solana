package codec_test

import (
	"encoding/json"
	"testing"
	"time"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
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
	modConfig := codeccommon.ModifiersConfig{
		&codeccommon.RenameModifierConfig{Fields: map[string]string{"Value": "V"}},
	}

	renameMod, err := modConfig.ToModifier(codec.DecoderHooks...)
	require.NoError(t, err)

	idlCodecWithMods, err := codec.NewNamedModifierCodec(idlCodec, codec.TestStructWithNestedStruct, renameMod)
	require.NoError(t, err)

	type modifiedTestStruct struct {
		V                uint8
		InnerStruct      codec.ObjectRef1
		BasicNestedArray [][]uint32
		Option           *string
		DefinedArray     []codec.ObjectRef2
		BasicVector      []string
		TimeVal          int64
		DurationVal      time.Duration
		PublicKey        ag_solana.PublicKey
		EnumVal          uint8
	}

	expected := modifiedTestStruct{
		V:                codec.DefaultTestStruct.Value,
		InnerStruct:      codec.DefaultTestStruct.InnerStruct,
		BasicNestedArray: codec.DefaultTestStruct.BasicNestedArray,
		Option:           codec.DefaultTestStruct.Option,
		DefinedArray:     codec.DefaultTestStruct.DefinedArray,
		BasicVector:      codec.DefaultTestStruct.BasicVector,
		TimeVal:          codec.DefaultTestStruct.TimeVal,
		DurationVal:      codec.DefaultTestStruct.DurationVal,
		PublicKey:        codec.DefaultTestStruct.PublicKey,
		EnumVal:          codec.DefaultTestStruct.EnumVal,
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
	require.Equal(t, expected.TimeVal, unmodifiedDecoded.TimeVal)
	require.Equal(t, expected.DurationVal, unmodifiedDecoded.DurationVal)
	require.Equal(t, expected.PublicKey, unmodifiedDecoded.PublicKey)
	require.Equal(t, expected.EnumVal, unmodifiedDecoded.EnumVal)
}

func TestNewIDLCodec_CircularDependency(t *testing.T) {
	t.Parallel()

	rawIDL := `{
		"accounts": [{
			"name": "TopLevelStruct",
			"type": {
				"kind": "struct",
				"fields": [{
					"name": "circularOne",
					"type": {
						"defined": "TypeOne"
					}
				}, {
					"name": "circularTwo",
					"type": {
						"defined": "TypeTwo"
					}
				}]
			}
		}],
		"types": [{
			"name": "TypeOne",
			"type": {
				"kind": "struct",
				"fields": [{
					"name": "circular",
					"type": {
						"defined": "TypeTwo"
					}
				}]
			}
		}, {
			"name": "TypeTwo",
			"type": {
				"kind": "struct",
				"fields": [{
					"name": "circular",
					"type": {
						"defined": "TypeOne"
					}
				}]
			}
		}]
	}`

	var idl codec.IDL
	if err := json.Unmarshal([]byte(rawIDL), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	_, err := codec.NewIDLCodec(idl)

	assert.ErrorIs(t, err, types.ErrInvalidConfig)
}
