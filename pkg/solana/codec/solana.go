package codec

import (
	"errors"

	"github.com/mitchellh/mapstructure"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

var (
	ErrUnsupported = errors.New("unsupported")
)

// BigIntHook allows *big.Int to be represented as any integer type or a string and to go back to them.
// Useful for config, or if when a model may use a go type that isn't a *big.Int when Pack expects one.
// Eg: int32 in a go struct from a plugin could require a *big.Int in Pack for int24, if it fits, we shouldn't care.
// SliceToArrayVerifySizeHook verifies that slices have the correct size when converting to an array
var DecoderHooks = []mapstructure.DecodeHookFunc{codec.BigIntHook, codec.SliceToArrayVerifySizeHook}

func NewNamedModifierCodec(original types.RemoteCodec, itemType string, modifier codec.Modifier) (types.RemoteCodec, error) {
	mod, err := codec.NewByItemTypeModifier(map[string]codec.Modifier{itemType: modifier})
	if err != nil {
		return nil, err
	}

	return codec.NewModifierCodec(original, mod, DecoderHooks...)
}

// NewIDLCodec is for Anchor custom types
func NewIDLCodec(idl IDL) (encodings.CodecFromTypeCodec, error) {
	builder := binary.LittleEndian()
	accounts := make(map[string]encodings.TypeCodec)
	codecs := make(map[string]encodings.TypeCodec)

	for _, account := range idl.Accounts {
		var (
			name     string
			accCodec encodings.TypeCodec
			err      error
		)

		name, accCodec, codecs, err = createNamedCodec(builder, account, codecs, idl.Types)
		if err != nil {
			return nil, err
		}

		accounts[name] = accCodec
	}

	return encodings.CodecFromTypeCodec(accounts), nil
}

func createNamedCodec(
	builder encodings.Builder,
	def IdlTypeDef,
	codecs map[string]encodings.TypeCodec,
	typeDefs IdlTypeDefSlice,
) (string, encodings.TypeCodec, map[string]encodings.TypeCodec, error) {
	caser := cases.Title(language.English)
	name := def.Name

	switch def.Type.Kind {
	case IdlTypeDefTyKindStruct:
		if def.Type.Fields == nil {
			return name, nil, nil, types.ErrInvalidEncoding
		}

		named := make([]encodings.NamedTypeCodec, len(*def.Type.Fields))

		for idx, field := range *def.Type.Fields {
			fieldName := field.Name

			typedCodec, err := processFieldType(field.Type, codecs, builder, typeDefs)
			if err != nil {
				return name, nil, nil, err
			}

			named[idx] = encodings.NamedTypeCodec{Name: caser.String(fieldName), Codec: typedCodec}
		}

		structCodec, err := encodings.NewStructCodec(named)
		if err != nil {
			return name, nil, nil, err
		}

		return name, structCodec, codecs, nil
	case IdlTypeDefTyKindEnum:
		// TODO: not yet sure how to handle enums
		// maybe as type map[string]interface{}??
		// enums in Rust can have properties or variants
		//
		// enums can also be simple uint8 values
		// a simple enum can be represented as map[uint8]struct{}
		fallthrough
	default:
		return name, nil, nil, types.ErrInvalidEncoding
	}
}

func processFieldType(idlType IdlType, codecs map[string]encodings.TypeCodec, builder encodings.Builder, typeDefs IdlTypeDefSlice) (encodings.TypeCodec, error) {
	switch true {
	case idlType.IsString():
		codec, err := getCodecByStringType(idlType.GetString(), 0, builder)
		if err != nil {
			return nil, err
		}

		return codec, nil
	case idlType.IsArray():
		idlArray := idlType.GetArray()

		codec, err := processFieldType(idlArray.Thing, codecs, builder, typeDefs)
		if err != nil {
			return nil, err
		}

		return encodings.NewArray(idlArray.Num, codec)
	case idlType.IsIdlTypeOption():
		// Go doesn't have an `Option` type; use pointer to type instead
		// this should be automatic in the codec
		opt := idlType.GetIdlTypeOption()

		codec, err := processFieldType(opt.Option, codecs, builder, typeDefs)
		if err != nil {
			return nil, err
		}

		return codec, nil
	case idlType.IsIdlTypeDefined():
		definedName := idlType.GetIdlTypeDefined()
		if definedName == nil {
			return nil, types.ErrInvalidEncoding
		}

		// already exists as a type in the typed codecs
		if savedCodec, ok := codecs[definedName.Defined]; ok {
			return savedCodec, nil
		}

		// codec by defined type doesn't exist
		// process it using the provided typeDefs
		nextDef := typeDefs.GetByName(definedName.Defined)
		if nextDef == nil {
			return nil, types.ErrInvalidEncoding
		}

		newTypeName, newTypeCodec, newCodecs, err := createNamedCodec(builder, *nextDef, codecs, typeDefs)
		if err != nil {
			return nil, err
		}

		// we know that recursive found codecs are types so add them to the type lookup
		codecs = newCodecs
		codecs[newTypeName] = newTypeCodec

		return newTypeCodec, nil
	case idlType.IsIdlTypeVec():
		// TODO: implement vector type
		// this should follow the same pattern as array, but the number of elements
		// is not known
		fallthrough
	default:
		return nil, ErrUnsupported
	}
}

func getCodecByStringType(curType IdlTypeAsString, len int, builder encodings.Builder) (encodings.TypeCodec, error) {
	switch curType {
	case IdlTypeBool:
		return builder.Bool(), nil
	case IdlTypeU8:
		return builder.Uint8(), nil
	case IdlTypeI8:
		return builder.Int8(), nil
	case IdlTypeU16:
		return builder.Uint16(), nil
	case IdlTypeI16:
		return builder.Int16(), nil
	case IdlTypeU32:
		return builder.Uint32(), nil
	case IdlTypeI32:
		return builder.Int32(), nil
	case IdlTypeU64:
		return builder.Uint64(), nil
	case IdlTypeI64:
		return builder.Int64(), nil
	case IdlTypeU128, IdlTypeI128:
		return builder.BigInt(16, true)
	case IdlTypeString:
		return builder.String(1000) // TODO: set max len from somewhere
	case IdlTypeUnixTimestamp:
		return NewUnixTimestamp(builder), nil
	case IdlTypeDuration:
		return NewDuration(builder), nil
	case IdlTypeBytes:
		return encodings.NewArray(len, builder.Uint8())
	// case IdlTypeHash:          nil, // TODO: a Hash is a wrapper for a PublicKey
	case IdlTypePublicKey:
		// TODO: still investigating
		// var pk ag_solana.PublicKey // [32]byte??
		// maybe there is a value for public key length??
		return nil, types.ErrInvalidEncoding
	default:
		return nil, types.ErrInvalidEncoding
	}
}
