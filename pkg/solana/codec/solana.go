/*
Package codec provides functions to create a codec from an Anchor IDL. All Anchor primitives map to the following native
Go values:

bool -> bool
string -> string
bytes -> []byte
[u|i][8-64] -> [u]int[8-64]
[u|i]128 -> *big.Int
duration -> time.Duration
unixTimestamp -> int64
publicKey -> [32]byte
hash -> [32]byte

Enums as an Anchor data structure are only supported in their basic form of uint8 values. Enums with variants are not
supported at this time.

Modifiers can be provided to assist in modifying property names, adding properties, etc.
*/
package codec

import (
	"fmt"
	"math"

	"github.com/mitchellh/mapstructure"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

const (
	DefaultHashBitLength = 32
	unknownIDLFormat     = "%w: unknown IDL type def %s"
)

// BigIntHook allows *big.Int to be represented as any integer type or a string and to go back to them.
// Useful for config, or if when a model may use a go type that isn't a *big.Int when Pack expects one.
// Eg: int32 in a go struct from a plugin could require a *big.Int in Pack for int24, if it fits, we shouldn't care.
// SliceToArrayVerifySizeHook verifies that slices have the correct size when converting to an array
// EpochToTimeHook allows multiple conversions: time.Time -> int64; int64 -> time.Time; *big.Int -> time.Time; and more
var DecoderHooks = []mapstructure.DecodeHookFunc{codec.EpochToTimeHook, codec.BigIntHook, codec.SliceToArrayVerifySizeHook}

func NewNamedModifierCodec(original types.RemoteCodec, itemType string, modifier codec.Modifier) (types.RemoteCodec, error) {
	mod, err := codec.NewByItemTypeModifier(map[string]codec.Modifier{itemType: modifier})
	if err != nil {
		return nil, err
	}

	modCodec, err := codec.NewModifierCodec(original, mod, DecoderHooks...)
	if err != nil {
		return nil, err
	}

	_, err = modCodec.CreateType(itemType, true)

	return modCodec, err
}

// NewIDLCodec is for Anchor custom types
func NewIDLCodec(idl IDL) (encodings.CodecFromTypeCodec, error) {
	accounts := make(map[string]encodings.TypeCodec)

	refs := &codecRefs{
		builder:  binary.LittleEndian(),
		codecs:   make(map[string]encodings.TypeCodec),
		typeDefs: idl.Types,
	}

	for _, account := range idl.Accounts {
		var (
			name     string
			accCodec encodings.TypeCodec
			err      error
		)

		name, accCodec, err = createNamedCodec(account, refs)
		if err != nil {
			return nil, err
		}

		accounts[name] = accCodec
	}

	return encodings.CodecFromTypeCodec(accounts), nil
}

type codecRefs struct {
	builder  encodings.Builder
	codecs   map[string]encodings.TypeCodec
	typeDefs IdlTypeDefSlice
}

func createNamedCodec(
	def IdlTypeDef,
	refs *codecRefs,
) (string, encodings.TypeCodec, error) {
	caser := cases.Title(language.English)
	name := def.Name

	switch def.Type.Kind {
	case IdlTypeDefTyKindStruct:
		return asStruct(def, refs, name, caser)
	case IdlTypeDefTyKindEnum:
		variants := def.Type.Variants
		if !variants.IsAllUint8() {
			return name, nil, fmt.Errorf("%w: variants are not supported", types.ErrInvalidConfig)
		}

		return name, refs.builder.Uint8(), nil
	default:
		return name, nil, fmt.Errorf(unknownIDLFormat, types.ErrInvalidConfig, def.Type.Kind)
	}
}

func asStruct(
	def IdlTypeDef,
	refs *codecRefs,
	name string,
	caser cases.Caser,
) (string, encodings.TypeCodec, error) {
	if def.Type.Fields == nil {
		return name, nil, fmt.Errorf("%w: provided def type fields should not be nil", types.ErrInvalidConfig)
	}

	named := make([]encodings.NamedTypeCodec, len(*def.Type.Fields))

	for idx, field := range *def.Type.Fields {
		fieldName := field.Name

		typedCodec, err := processFieldType(field.Type, refs)
		if err != nil {
			return name, nil, err
		}

		named[idx] = encodings.NamedTypeCodec{Name: caser.String(fieldName), Codec: typedCodec}
	}

	structCodec, err := encodings.NewStructCodec(named)
	if err != nil {
		return name, nil, err
	}

	return name, structCodec, nil
}

func processFieldType(idlType IdlType, refs *codecRefs) (encodings.TypeCodec, error) {
	switch true {
	case idlType.IsString():
		return getCodecByStringType(idlType.GetString(), refs.builder)
	case idlType.IsIdlTypeOption():
		return asOption(idlType.GetIdlTypeOption(), refs)
	case idlType.IsIdlTypeDefined():
		return asDefined(idlType.GetIdlTypeDefined(), refs)
	case idlType.IsArray():
		return asArray(idlType.GetArray(), refs)
	case idlType.IsIdlTypeVec():
		return asVec(idlType.GetIdlTypeVec(), refs)
	default:
		return nil, fmt.Errorf("%w: unknown IDL type def", types.ErrInvalidConfig)
	}
}

func asOption(opt *IdlTypeOption, refs *codecRefs) (encodings.TypeCodec, error) {
	// Go doesn't have an `Option` type; use pointer to type instead
	// this should be automatic in the codec
	codec, err := processFieldType(opt.Option, refs)
	if err != nil {
		return nil, err
	}

	return codec, nil
}

func asDefined(definedName *IdlTypeDefined, refs *codecRefs) (encodings.TypeCodec, error) {
	if definedName == nil {
		return nil, fmt.Errorf("%w: defined type name should not be nil", types.ErrInvalidConfig)
	}

	// already exists as a type in the typed codecs
	if savedCodec, ok := refs.codecs[definedName.Defined]; ok {
		return savedCodec, nil
	}

	// codec by defined type doesn't exist
	// process it using the provided typeDefs
	nextDef := refs.typeDefs.GetByName(definedName.Defined)
	if nextDef == nil {
		return nil, fmt.Errorf("%w: IDL type does not exist for name %s", types.ErrInvalidConfig, definedName.Defined)
	}

	newTypeName, newTypeCodec, err := createNamedCodec(*nextDef, refs)
	if err != nil {
		return nil, err
	}

	// we know that recursive found codecs are types so add them to the type lookup
	refs.codecs[newTypeName] = newTypeCodec

	return newTypeCodec, nil
}

func asArray(idlArray *IdlTypeArray, refs *codecRefs) (encodings.TypeCodec, error) {
	codec, err := processFieldType(idlArray.Thing, refs)
	if err != nil {
		return nil, err
	}

	return encodings.NewArray(idlArray.Num, codec)
}

func asVec(idlVec *IdlTypeVec, refs *codecRefs) (encodings.TypeCodec, error) {
	codec, err := processFieldType(idlVec.Vec, refs)
	if err != nil {
		return nil, err
	}

	b, err := refs.builder.Int(4)
	if err != nil {
		return nil, err
	}

	return encodings.NewSlice(codec, b)
}

func getCodecByStringType(curType IdlTypeAsString, builder encodings.Builder) (encodings.TypeCodec, error) {
	switch curType {
	case IdlTypeBool:
		return builder.Bool(), nil
	case IdlTypeString:
		return builder.String(math.MaxUint32)
	case IdlTypeI8, IdlTypeI16, IdlTypeI32, IdlTypeI64, IdlTypeI128:
		return getIntCodecByStringType(curType, builder)
	case IdlTypeU8, IdlTypeU16, IdlTypeU32, IdlTypeU64, IdlTypeU128:
		return getUIntCodecByStringType(curType, builder)
	case IdlTypeUnixTimestamp, IdlTypeDuration:
		return getTimeCodecByStringType(curType, builder)
	case IdlTypeBytes, IdlTypePublicKey, IdlTypeHash:
		return getByteCodecByStringType(curType, builder)
	default:
		return nil, fmt.Errorf(unknownIDLFormat, types.ErrInvalidConfig, curType)
	}
}

func getIntCodecByStringType(curType IdlTypeAsString, builder encodings.Builder) (encodings.TypeCodec, error) {
	switch curType {
	case IdlTypeI8:
		return builder.Int8(), nil
	case IdlTypeI16:
		return builder.Int16(), nil
	case IdlTypeI32:
		return builder.Int32(), nil
	case IdlTypeI64:
		return builder.Int64(), nil
	case IdlTypeI128:
		return builder.BigInt(16, true)
	default:
		return nil, fmt.Errorf(unknownIDLFormat, types.ErrInvalidConfig, curType)
	}
}

func getUIntCodecByStringType(curType IdlTypeAsString, builder encodings.Builder) (encodings.TypeCodec, error) {
	switch curType {
	case IdlTypeU8:
		return builder.Uint8(), nil
	case IdlTypeU16:
		return builder.Uint16(), nil
	case IdlTypeU32:
		return builder.Uint32(), nil
	case IdlTypeU64:
		return builder.Uint64(), nil
	case IdlTypeU128:
		return builder.BigInt(16, true)
	default:
		return nil, fmt.Errorf(unknownIDLFormat, types.ErrInvalidConfig, curType)
	}
}

func getTimeCodecByStringType(curType IdlTypeAsString, builder encodings.Builder) (encodings.TypeCodec, error) {
	switch curType {
	case IdlTypeUnixTimestamp:
		return builder.Int64(), nil
	case IdlTypeDuration:
		return NewDuration(builder), nil
	default:
		return nil, fmt.Errorf(unknownIDLFormat, types.ErrInvalidConfig, curType)
	}
}

func getByteCodecByStringType(curType IdlTypeAsString, builder encodings.Builder) (encodings.TypeCodec, error) {
	switch curType {
	case IdlTypeBytes:
		b, err := builder.Int(4)
		if err != nil {
			return nil, err
		}

		return encodings.NewSlice(builder.Uint8(), b)
	case IdlTypePublicKey, IdlTypeHash:
		return encodings.NewArray(DefaultHashBitLength, builder.Uint8())
	default:
		return nil, fmt.Errorf(unknownIDLFormat, types.ErrInvalidConfig, curType)
	}
}
