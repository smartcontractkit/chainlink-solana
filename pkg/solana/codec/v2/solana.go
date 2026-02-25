package codecv2

import (
	"fmt"
	"math"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/gagliardetto/anchor-go/idl/idltype"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

const (
	DefaultHashBitLength = 32
	unknownIDLFormat     = "%w: unknown IDL type def %q"
)

func CreateCodecEntry(idlDefinition interface{}, offChainName string, idl anchoridl.Idl, mod commoncodec.Modifier) (entry solcommoncodec.Entry, err error) {
	switch v := idlDefinition.(type) {
	// No IdlTypeDef, PDATypeDef required as chain_reader (codec based) has moved to chain_accessor (binding based)
	case anchoridl.IdlInstruction:
		entry, err = NewInstructionArgsEntry(offChainName, InstructionArgsIDLTypes{Instruction: v, Types: idl.Types}, mod, binary.LittleEndian())
	case anchoridl.IdlEvent:
		entry, err = NewEventArgsEntry(offChainName, EventIDLTypes{Event: v, Types: idl.Types}, true, mod, binary.LittleEndian())
	default:
		return nil, fmt.Errorf("unknown codec IDL definition: %T", idlDefinition)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create %q codec entry: %w", offChainName, err)
	}

	return entry, nil
}

func FindDefinitionFromIDL(cfgType solcommoncodec.ChainConfigType, chainSpecificName string, idl anchoridl.Idl) (interface{}, error) {
	// not the most efficient way to do this, but these slices should always be very, very small
	switch cfgType {
	case solcommoncodec.ChainConfigTypeAccountDef:
		// codecv2 does not support accounts
		return nil, fmt.Errorf("codecv2 does not support accounts: %q", chainSpecificName)

	case solcommoncodec.ChainConfigTypeInstructionDef:
		for i := range idl.Instructions {
			if idl.Instructions[i].Name == chainSpecificName {
				return idl.Instructions[i], nil
			}
		}
		return nil, fmt.Errorf("failed to find instruction %q in IDL", chainSpecificName)

	case solcommoncodec.ChainConfigTypeEventDef:
		for i := range idl.Events {
			if idl.Events[i].Name == chainSpecificName {
				return idl.Events[i], nil
			}
		}
		return nil, fmt.Errorf("failed to find event %q in IDL", chainSpecificName)
	}
	return nil, fmt.Errorf("unknown type: %q", cfgType)
}

// ExtractEventIDL extracts an event definition from the IDL by name.
func ExtractEventIDL(eventName string, idl anchoridl.Idl) (anchoridl.IdlEvent, error) {
	idlDef, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, eventName, idl)
	if err != nil {
		return anchoridl.IdlEvent{}, err
	}
	eventIdl, isOk := idlDef.(anchoridl.IdlEvent)
	if !isOk {
		return anchoridl.IdlEvent{}, fmt.Errorf("unexpected type from IDL definition for event read: %q", eventName)
	}
	return eventIdl, nil
}

type codecRefs struct {
	builder      commonencodings.Builder
	codecs       map[string]commonencodings.TypeCodec
	typeDefs     anchoridl.IdTypeDef_slice
	dependencies map[string][]string
}

func createCodecType(
	def anchoridl.IdlTypeDef,
	refs *codecRefs,
) (string, commonencodings.TypeCodec, error) {
	name := def.Name
	// as opposed to codecv1, def.Ty is an interface instead of a concrete type
	// hence we cannot access def.Ty.Kind directly
	switch vv := def.Ty.(type) {
	case *anchoridl.IdlTypeDefTyStruct:
		return asStruct(refs, name)
	case *anchoridl.IdlTypeDefTyEnum:
		variants := vv.Variants
		if !variants.IsAllSimple() {
			return name, nil, fmt.Errorf("%w: variants are not supported", commontypes.ErrInvalidConfig)
		}
		return name, refs.builder.Uint8(), nil
	// This is only being used with chain_reader, which will be disabled with the anchor upgrade
	// case IdlTypeDefTyKindCustom:
	// 	switch def.Type.Codec {
	// 	case "onramp_address":
	// 		return name, NewOnRampAddress(refs.builder), nil
	// 	case "cross_chain_amount":
	// 		return name, NewCrossChainAmount(), nil
	// 	default:
	// 		return name, nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, def.Type.Codec)
	// 	}
	default:
		return name, nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, name)
	}
}

func asStruct(
	refs *codecRefs,
	name string, // name is the struct name and can be used in dependency checks
) (string, commonencodings.TypeCodec, error) {
	namedType := refs.typeDefs.ByName(name)
	if namedType == nil {
		return name, nil, fmt.Errorf("named type %q not found", name)
	}
	// as opposed to codecv1, namedType.Ty is an interface instead of a concrete type
	// hence we cannot access def.Ty.Kind or def.Ty.Fields directly
	var structType anchoridl.IdlTypeDefTyStruct
	switch vv := namedType.Ty.(type) {
	case *anchoridl.IdlTypeDefTyStruct:
		structType = *vv
	default:
		return name, nil, fmt.Errorf("unhandled type: %T", vv)
	}

	// as opposed to codecv1, IdlTypeDefTyStruct.Fields is an interface instead of a concrete type
	switch fields := structType.Fields.(type) {
	case anchoridl.IdlDefinedFieldsNamed:
		named := make([]commonencodings.NamedTypeCodec, len(fields))
		for idx, field := range fields {
			fieldName := field.Name

			// name here is the parent type name
			// field.Name is the field name corresponding to field.Ty
			// we pass in the parent type name to the processFieldType function to handle the case where the field is a defined type
			// and to check for circular dependencies
			typedCodec, err := processFieldType(name, field.Ty, refs)
			if err != nil {
				return name, nil, err
			}

			named[idx] = commonencodings.NamedTypeCodec{Name: cases.Title(language.English, cases.NoLower).String(fieldName), Codec: typedCodec}
		}
		structCodec, err := commonencodings.NewStructCodec(named)
		if err != nil {
			return name, nil, err
		}

		return name, structCodec, nil
	default:
		return name, nil, fmt.Errorf("unhandled type: %T", fields)
	}
}

func asStructForInstructionArgs(
	fields []anchoridl.IdlField,
	refs *codecRefs,
	ixName string, // the name of the argument struct
) (commonencodings.TypeCodec, error) {
	named := make([]commonencodings.NamedTypeCodec, len(fields))

	for idx, field := range fields {
		fieldName := field.Name
		typedCodec, err := processFieldType(ixName, field.Ty, refs)
		if err != nil {
			return nil, err
		}
		named[idx] = commonencodings.NamedTypeCodec{Name: cases.Title(language.English, cases.NoLower).String(fieldName), Codec: typedCodec}
	}

	var isVecOrArray bool
	if len(fields) > 0 {
		switch fields[0].Ty.(type) {
		case *idltype.Vec:
			isVecOrArray = true
		case *idltype.Array:
			isVecOrArray = true
		default:
			isVecOrArray = false
		}
	}

	// If it's an instruction arg that's just a single array/vec → return the array codec directly (no struct wrapper)
	if len(named) == 1 && isVecOrArray {
		return named[0].Codec, nil
	}

	structCodec, err := commonencodings.NewStructCodec(named)
	if err != nil {
		return nil, err
	}

	return structCodec, nil
}

func processFieldType(parentTypeName string, idlType idltype.IdlType, refs *codecRefs) (commonencodings.TypeCodec, error) {
	// Use type switch for all types
	switch t := idlType.(type) {
	case *idltype.String:
		return refs.builder.String(math.MaxUint32)
	case *idltype.Bool:
		return refs.builder.Bool(), nil
	// integer types
	case *idltype.I8:
		return refs.builder.Int8(), nil
	case *idltype.I16:
		return refs.builder.Int16(), nil
	case *idltype.I32:
		return refs.builder.Int32(), nil
	case *idltype.I64:
		return refs.builder.Int64(), nil
	case *idltype.I128:
		return refs.builder.BigInt(16, true)
	// unsigned integer types
	case *idltype.U8:
		return refs.builder.Uint8(), nil
	case *idltype.U16:
		return refs.builder.Uint16(), nil
	case *idltype.U32:
		return refs.builder.Uint32(), nil
	case *idltype.U64:
		return refs.builder.Uint64(), nil
	case *idltype.U128:
		return refs.builder.BigInt(16, false)
	case *idltype.Bytes:
		b, err := refs.builder.Int(4)
		if err != nil {
			return nil, err
		}
		return commonencodings.NewSlice(refs.builder.Uint8(), b)
	case *idltype.Pubkey:
		return commonencodings.NewArray(DefaultHashBitLength, refs.builder.Uint8())
	case *idltype.Defined:
		return asDefined(parentTypeName, t, refs)
	case *idltype.Array:
		return asArray(parentTypeName, t, refs)
	case *idltype.Vec:
		return asVec(parentTypeName, t, refs)
	case *idltype.Option:
		// Go doesn't have an `Option` type; use pointer to type instead
		inner, err := processFieldType(parentTypeName, t.Option, refs)
		if err != nil {
			return nil, err
		}
		return solcommoncodec.NewOption(inner), nil
	default:
		// Handle custom types by checking the string representation
		typeName := idlType.String()
		switch typeName {
		case "hash":
			return commonencodings.NewArray(DefaultHashBitLength, refs.builder.Uint8())
		case "unixTimestamp":
			return refs.builder.Int64(), nil
		case "duration":
			return solcommoncodec.NewDuration(refs.builder), nil
		default:
			return nil, fmt.Errorf("%w: unknown IDL type def %q", commontypes.ErrInvalidConfig, typeName)
		}
	}
}

func asDefined(parentTypeName string, definedType *idltype.Defined, refs *codecRefs) (commonencodings.TypeCodec, error) {
	definedName := definedType.Name

	// already exists as a type in the typed codecs
	if savedCodec, ok := refs.codecs[definedName]; ok {
		return savedCodec, nil
	}

	// nextDef should not have a dependency on definedName
	if !validDependency(refs, parentTypeName, definedName) {
		return nil, fmt.Errorf("%w: circular dependency detected on %q -> %q relation", commontypes.ErrInvalidConfig, parentTypeName, definedName)
	}

	// codec by defined type doesn't exist
	// process it using the provided typeDefs
	nextDef := refs.typeDefs.ByName(definedName)
	if nextDef == nil {
		return nil, fmt.Errorf("%w: IDL type does not exist for name %q", commontypes.ErrInvalidConfig, definedName)
	}

	saveDependency(refs, parentTypeName, definedName)

	newTypeName, newTypeCodec, err := createCodecType(*nextDef, refs)
	if err != nil {
		return nil, err
	}

	// we know that recursive found codecs are types so add them to the type lookup
	refs.codecs[newTypeName] = newTypeCodec

	return newTypeCodec, nil
}

func asArray(parentTypeName string, idlArray *idltype.Array, refs *codecRefs) (commonencodings.TypeCodec, error) {
	if idlArray == nil {
		return nil, fmt.Errorf("%w: array type cannot be nil", commontypes.ErrInvalidConfig)
	}

	// idlArray.Type is the inner type as idltype.IdlType
	// idlArray.Size is the length (IdlArrayLen interface - can be IdlArrayLenValue or IdlArrayLenGeneric)
	innerType := idlArray.Type

	// Handle array length - could be a value or a generic
	var lenInt int
	switch size := idlArray.Size.(type) {
	case *idltype.IdlArrayLenValue:
		lenInt = size.Value
	case *idltype.IdlArrayLenGeneric:
		return nil, fmt.Errorf("%w: generic array lengths are not supported yet: %s", commontypes.ErrInvalidConfig, size.Generic)
	default:
		return nil, fmt.Errorf("%w: unknown array length type: %T", commontypes.ErrInvalidConfig, idlArray.Size)
	}

	// better to implement bytes to big int codec modifiers, but this works fine
	if lenInt == 28 && innerType.String() == "u8" {
		// nolint:gosec
		// G115: integer overflow conversion int -&gt; uint
		return binary.BigEndian().BigInt(uint(lenInt), false)
	}

	// Process the inner type recursively
	codec, err := processFieldType(parentTypeName, innerType, refs)
	if err != nil {
		return nil, err
	}

	return commonencodings.NewArray(lenInt, codec)
}

func asVec(parentTypeName string, idlVec *idltype.Vec, refs *codecRefs) (commonencodings.TypeCodec, error) {
	codec, err := processFieldType(parentTypeName, idlVec.Vec, refs)
	if err != nil {
		return nil, err
	}

	b, err := refs.builder.Int(4)
	if err != nil {
		return nil, err
	}

	return commonencodings.NewSlice(codec, b)
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
