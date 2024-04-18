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

// NewIDLAccountCodec is for Anchor custom types
func NewIDLAccountCodec(idl IDL, builder encodings.Builder) (types.RemoteCodec, error) {
	return newIDLCoded(idl, builder, idl.Accounts, true)
}

func NewIDLDefinedTypesCodec(idl IDL, builder encodings.Builder) (types.RemoteCodec, error) {
	return newIDLCoded(idl, builder, idl.Types, false)
}

func newIDLCoded(
	idl IDL, builder encodings.Builder, from IdlTypeDefSlice, includeDiscriminator bool) (types.RemoteCodec, error) {
	typeCodecs := make(encodings.LenientCodecFromTypeCodec)

	refs := &codecRefs{
		builder:      builder,
		codecs:       make(map[string]encodings.TypeCodec),
		typeDefs:     idl.Types,
		dependencies: make(map[string][]string),
	}

	for _, def := range from {
		var (
			name     string
			accCodec encodings.TypeCodec
			err      error
		)

		name, accCodec, err = createNamedCodec(def, refs, includeDiscriminator)
		if err != nil {
			return nil, err
		}

		typeCodecs[name] = accCodec
	}

	return typeCodecs, nil
}

type codecRefs struct {
	builder      encodings.Builder
	codecs       map[string]encodings.TypeCodec
	typeDefs     IdlTypeDefSlice
	dependencies map[string][]string
}

func createNamedCodec(
	def IdlTypeDef,
	refs *codecRefs,
	includeDiscriminator bool,
) (string, encodings.TypeCodec, error) {
	caser := cases.Title(language.English)
	name := def.Name

	switch def.Type.Kind {
	case IdlTypeDefTyKindStruct:
		return asStruct(def, refs, name, caser, includeDiscriminator)
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
	name string, // name is the struct name and can be used in dependency checks
	caser cases.Caser,
	includeDiscriminator bool,
) (string, encodings.TypeCodec, error) {
	desLen := 0
	if includeDiscriminator {
		desLen = 1
	}
	named := make([]encodings.NamedTypeCodec, len(*def.Type.Fields)+desLen)

	if includeDiscriminator {
		named[0] = encodings.NamedTypeCodec{Name: "Discriminator" + name, Codec: NewDiscriminator(name)}
	}

	for idx, field := range *def.Type.Fields {
		fieldName := field.Name

		typedCodec, err := processFieldType(name, field.Type, refs)
		if err != nil {
			return name, nil, err
		}

		named[idx+desLen] = encodings.NamedTypeCodec{Name: caser.String(fieldName), Codec: typedCodec}
	}

	structCodec, err := encodings.NewStructCodec(named)
	if err != nil {
		return name, nil, err
	}

	return name, structCodec, nil
}

func processFieldType(parentTypeName string, idlType IdlType, refs *codecRefs) (encodings.TypeCodec, error) {
	switch true {
	case idlType.IsString():
		return getCodecByStringType(idlType.GetString(), refs.builder)
	case idlType.IsIdlTypeOption():
		// Go doesn't have an `Option` type; use pointer to type instead
		// this should be automatic in the codec
		return processFieldType(parentTypeName, idlType.GetIdlTypeOption().Option, refs)
	case idlType.IsIdlTypeDefined():
		return asDefined(parentTypeName, idlType.GetIdlTypeDefined(), refs)
	case idlType.IsArray():
		return asArray(parentTypeName, idlType.GetArray(), refs)
	case idlType.IsIdlTypeVec():
		return asVec(parentTypeName, idlType.GetIdlTypeVec(), refs)
	default:
		return nil, fmt.Errorf("%w: unknown IDL type def", types.ErrInvalidConfig)
	}
}

func asDefined(parentTypeName string, definedName *IdlTypeDefined, refs *codecRefs) (encodings.TypeCodec, error) {
	if definedName == nil {
		return nil, fmt.Errorf("%w: defined type name should not be nil", types.ErrInvalidConfig)
	}

	// already exists as a type in the typed codecs
	if savedCodec, ok := refs.codecs[definedName.Defined]; ok {
		return savedCodec, nil
	}

	// nextDef should not have a dependency on definedName
	if !validDependency(refs, parentTypeName, definedName.Defined) {
		return nil, fmt.Errorf("%w: circular dependency detected on %s -> %s relation", types.ErrInvalidConfig, parentTypeName, definedName.Defined)
	}

	// codec by defined type doesn't exist
	// process it using the provided typeDefs
	nextDef := refs.typeDefs.GetByName(definedName.Defined)
	if nextDef == nil {
		return nil, fmt.Errorf("%w: IDL type does not exist for name %s", types.ErrInvalidConfig, definedName.Defined)
	}

	saveDependency(refs, parentTypeName, definedName.Defined)

	newTypeName, newTypeCodec, err := createNamedCodec(*nextDef, refs, false)
	if err != nil {
		return nil, err
	}

	// we know that recursive found codecs are types so add them to the type lookup
	refs.codecs[newTypeName] = newTypeCodec

	return newTypeCodec, nil
}

func asArray(parentTypeName string, idlArray *IdlTypeArray, refs *codecRefs) (encodings.TypeCodec, error) {
	codec, err := processFieldType(parentTypeName, idlArray.Thing, refs)
	if err != nil {
		return nil, err
	}

	return encodings.NewArray(idlArray.Num, codec)
}

func asVec(parentTypeName string, idlVec *IdlTypeVec, refs *codecRefs) (encodings.TypeCodec, error) {
	codec, err := processFieldType(parentTypeName, idlVec.Vec, refs)
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

func validDependency(refs *codecRefs, parent, child string) bool {
	deps, ok := refs.dependencies[child]
	if ok {
		for _, dep := range deps {
			if dep == parent {
				return false
			}
		}
	}

	return true
}

func saveDependency(refs *codecRefs, parent, child string) {
	deps, ok := refs.dependencies[parent]
	if !ok {
		deps = make([]string, 0)
	}

	refs.dependencies[parent] = append(deps, child)
}
