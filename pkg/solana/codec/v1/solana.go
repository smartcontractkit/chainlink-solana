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
package codecv1

import (
	"encoding/json"
	"fmt"
	"math"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	basecodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

const (
	DefaultHashBitLength = 32
	unknownIDLFormat     = "%w: unknown IDL type def %q"
)

// NewCodec creates a new [commontypes.RemoteCodec] for Solana.
func NewCodec(conf solcommoncodec.Config) (commontypes.RemoteCodec, error) {
	parsed := &solcommoncodec.ParsedTypes{
		EncoderDefs: map[string]solcommoncodec.Entry{},
		DecoderDefs: map[string]solcommoncodec.Entry{},
	}

	for offChainName, cfg := range conf.Configs {
		var idl IDL
		if err := json.Unmarshal([]byte(cfg.IDL), &idl); err != nil {
			return nil, err
		}

		mod, err := cfg.ModifierConfigs.ToModifier(solcommoncodec.DecoderHooks...)
		if err != nil {
			return nil, err
		}

		definition, err := FindDefinitionFromIDL(cfg.Type, cfg.ChainSpecificName, idl)
		if err != nil {
			return nil, err
		}

		cEntry, err := CreateCodecEntry(definition, offChainName, idl, mod)
		if err != nil {
			return nil, err
		}

		parsed.EncoderDefs["input."+offChainName] = cEntry
		parsed.DecoderDefs["output."+offChainName] = cEntry
	}

	return parsed.ToCodec()
}

func CreateCodecEntry(idlDefinition interface{}, offChainName string, idl IDL, mod basecodec.Modifier) (entry solcommoncodec.Entry, err error) {
	switch v := idlDefinition.(type) {
	case IdlTypeDef:
		entry, err = NewAccountEntry(offChainName, AccountIDLTypes{Account: v, Types: idl.Types}, true, mod, binary.LittleEndian())
	case IdlInstruction:
		entry, err = NewInstructionArgsEntry(offChainName, InstructionArgsIDLTypes{Instruction: v, Types: idl.Types}, mod, binary.LittleEndian())
	case IdlEvent:
		entry, err = NewEventArgsEntry(offChainName, EventIDLTypes{Event: v, Types: idl.Types}, true, mod, binary.LittleEndian())
	case PDATypeDef:
		entry, err = NewPDAEntry(offChainName, v, mod, binary.LittleEndian())
	default:
		return nil, fmt.Errorf("unknown codec IDL definition: %T", idlDefinition)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create %q codec entry: %w", offChainName, err)
	}

	return entry, nil
}

func FindDefinitionFromIDL(cfgType solcommoncodec.ChainConfigType, chainSpecificName string, idl IDL) (interface{}, error) {
	// not the most efficient way to do this, but these slices should always be very, very small
	switch cfgType {
	case solcommoncodec.ChainConfigTypeAccountDef:
		for i := range idl.Accounts {
			if idl.Accounts[i].Name == chainSpecificName {
				return idl.Accounts[i], nil
			}
		}
		return nil, fmt.Errorf("failed to find account %q in IDL", chainSpecificName)

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
func ExtractEventIDL(eventName string, idl IDL) (IdlEvent, error) {
	idlDef, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, eventName, idl)
	if err != nil {
		return IdlEvent{}, err
	}
	eventIdl, isOk := idlDef.(IdlEvent)
	if !isOk {
		return IdlEvent{}, fmt.Errorf("unexpected type from IDL definition for event read: %q", eventName)
	}
	return eventIdl, nil
}

// TODO Deprecate and remove this.
func NewIDLAccountCodec(idl IDL, builder commonencodings.Builder) (commontypes.RemoteCodec, error) {
	return newIDLCoded(idl, builder, idl.Accounts, true)
}

func NewNamedModifierCodec(original commontypes.RemoteCodec, itemType string, modifier basecodec.Modifier) (commontypes.RemoteCodec, error) {
	mod, err := basecodec.NewByItemTypeModifier(map[string]basecodec.Modifier{itemType: modifier})
	if err != nil {
		return nil, err
	}

	modCodec, err := basecodec.NewModifierCodec(original, mod, solcommoncodec.DecoderHooks...)
	if err != nil {
		return nil, err
	}

	_, err = modCodec.CreateType(itemType, true)

	return modCodec, err
}

// TODO Deprecate and remove this.
func NewIDLDefinedTypesCodec(idl IDL, builder commonencodings.Builder) (commontypes.RemoteCodec, error) {
	return newIDLCoded(idl, builder, idl.Types, false)
}

func newIDLCoded(
	idl IDL, builder commonencodings.Builder, from IdlTypeDefSlice, includeDiscriminator bool) (commontypes.RemoteCodec, error) {
	typeCodecs := make(commonencodings.LenientCodecFromTypeCodec)

	refs := &codecRefs{
		builder:      builder,
		codecs:       make(map[string]commonencodings.TypeCodec),
		typeDefs:     idl.Types,
		dependencies: make(map[string][]string),
	}

	for _, def := range from {
		var (
			name     string
			accCodec commonencodings.TypeCodec
			err      error
		)

		name, accCodec, err = createCodecType(def, refs, includeDiscriminator)
		if err != nil {
			return nil, err
		}

		typeCodecs[name] = accCodec
	}

	return typeCodecs, nil
}

type codecRefs struct {
	builder      commonencodings.Builder
	codecs       map[string]commonencodings.TypeCodec
	typeDefs     IdlTypeDefSlice
	dependencies map[string][]string
}

func createCodecType(
	def IdlTypeDef,
	refs *codecRefs,
	// TODO Deprecated includeDiscriminator is not needed here after NewIDLAccountCodec gets cleaned up
	includeDiscriminator bool,
) (string, commonencodings.TypeCodec, error) {
	name := def.Name
	switch def.Type.Kind {
	case IdlTypeDefTyKindStruct:
		return asStruct(*def.Type.Fields, refs, name, includeDiscriminator, false)
	case IdlTypeDefTyKindEnum:
		variants := def.Type.Variants
		if !variants.IsAllUint8() {
			return name, nil, fmt.Errorf("%w: variants are not supported", commontypes.ErrInvalidConfig)
		}
		return name, refs.builder.Uint8(), nil
	case IdlTypeDefTyKindCustom:
		switch def.Type.Codec {
		case "onramp_address":
			return name, NewOnRampAddress(refs.builder), nil
		case "cross_chain_amount":
			return name, solcommoncodec.NewCrossChainAmount(), nil
		default:
			return name, nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, def.Type.Codec)
		}
	default:
		return name, nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, def.Type.Kind)
	}
}

func asStruct(
	fields []IdlField,
	refs *codecRefs,
	name string, // name is the struct name and can be used in dependency checks
	// TODO Deprecated includeDiscriminator is not needed here after NewIDLAccountCodec gets cleaned up
	includeDiscriminator bool,
	isInstructionArgs bool,
) (string, commonencodings.TypeCodec, error) {
	desLen := 0
	if includeDiscriminator {
		desLen = 1
	}

	named := make([]commonencodings.NamedTypeCodec, len(fields)+desLen)

	if includeDiscriminator {
		named[0] = commonencodings.NamedTypeCodec{Name: "Discriminator" + name, Codec: solcommoncodec.NewDiscriminator(name, true)}
	}

	for idx, field := range fields {
		fieldName := field.Name

		typedCodec, err := processFieldType(name, field.Type, refs)
		if err != nil {
			return name, nil, err
		}

		named[idx+desLen] = commonencodings.NamedTypeCodec{Name: cases.Title(language.English, cases.NoLower).String(fieldName), Codec: typedCodec}
	}

	// accounts have to be in a struct, instruction args don't if they're an array
	if len(named) == 1 && isInstructionArgs && (fields[0].Type.IsIdlTypeVec() || fields[0].Type.IsArray()) {
		return name, named[0].Codec, nil
	}

	structCodec, err := commonencodings.NewStructCodec(named)
	if err != nil {
		return name, nil, err
	}

	return name, structCodec, nil
}

func processFieldType(parentTypeName string, idlType IdlType, refs *codecRefs) (commonencodings.TypeCodec, error) {
	switch true {
	case idlType.IsString():
		return getCodecByStringType(idlType.GetString(), refs.builder)
	case idlType.IsIdlTypeOption():
		// Go doesn't have an `Option` type; use pointer to type instead
		inner, err := processFieldType(parentTypeName, idlType.GetIdlTypeOption().Option, refs)
		return solcommoncodec.NewOption(inner), err
	case idlType.IsIdlTypeDefined():
		return asDefined(parentTypeName, idlType.GetIdlTypeDefined(), refs)
	case idlType.IsArray():
		return asArray(parentTypeName, idlType.GetArray(), refs)
	case idlType.IsIdlTypeVec():
		return asVec(parentTypeName, idlType.GetIdlTypeVec(), refs)
	default:
		return nil, fmt.Errorf("%w: unknown IDL type def", commontypes.ErrInvalidConfig)
	}
}

func asDefined(parentTypeName string, definedName *IdlTypeDefined, refs *codecRefs) (commonencodings.TypeCodec, error) {
	if definedName == nil {
		return nil, fmt.Errorf("%w: defined type name should not be nil", commontypes.ErrInvalidConfig)
	}

	// already exists as a type in the typed codecs
	if savedCodec, ok := refs.codecs[definedName.Defined]; ok {
		return savedCodec, nil
	}

	// nextDef should not have a dependency on definedName
	if !validDependency(refs, parentTypeName, definedName.Defined) {
		return nil, fmt.Errorf("%w: circular dependency detected on %q -> %q relation", commontypes.ErrInvalidConfig, parentTypeName, definedName.Defined)
	}

	// codec by defined type doesn't exist
	// process it using the provided typeDefs
	nextDef := refs.typeDefs.GetByName(definedName.Defined)
	if nextDef == nil {
		return nil, fmt.Errorf("%w: IDL type does not exist for name %q", commontypes.ErrInvalidConfig, definedName.Defined)
	}

	saveDependency(refs, parentTypeName, definedName.Defined)

	newTypeName, newTypeCodec, err := createCodecType(*nextDef, refs, false)
	if err != nil {
		return nil, err
	}

	// we know that recursive found codecs are types so add them to the type lookup
	refs.codecs[newTypeName] = newTypeCodec

	return newTypeCodec, nil
}

func asArray(parentTypeName string, idlArray *IdlTypeArray, refs *codecRefs) (commonencodings.TypeCodec, error) {
	if idlArray == nil {
		return nil, fmt.Errorf("%w: field type cannot be nil", commontypes.ErrInvalidConfig)
	}

	// better to implement bytes to big int codec modifiers, but this works fine
	if idlArray.Num == 28 && idlArray.Thing.AsString == IdlTypeU8 {
		// nolint:gosec
		// G115: integer overflow conversion int -&gt; uint
		return binary.BigEndian().BigInt(uint(idlArray.Num), false)
	}

	codec, err := processFieldType(parentTypeName, idlArray.Thing, refs)
	if err != nil {
		return nil, err
	}

	return commonencodings.NewArray(idlArray.Num, codec)
}

func asVec(parentTypeName string, idlVec *IdlTypeVec, refs *codecRefs) (commonencodings.TypeCodec, error) {
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

func getCodecByStringType(curType IdlTypeAsString, builder commonencodings.Builder) (commonencodings.TypeCodec, error) {
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
		return nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, curType)
	}
}

func getIntCodecByStringType(curType IdlTypeAsString, builder commonencodings.Builder) (commonencodings.TypeCodec, error) {
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
		return nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, curType)
	}
}

func getUIntCodecByStringType(curType IdlTypeAsString, builder commonencodings.Builder) (commonencodings.TypeCodec, error) {
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
		return builder.BigInt(16, false)
	default:
		return nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, curType)
	}
}

func getTimeCodecByStringType(curType IdlTypeAsString, builder commonencodings.Builder) (commonencodings.TypeCodec, error) {
	switch curType {
	case IdlTypeUnixTimestamp:
		return builder.Int64(), nil
	case IdlTypeDuration:
		return solcommoncodec.NewDuration(builder), nil
	default:
		return nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, curType)
	}
}

func getByteCodecByStringType(curType IdlTypeAsString, builder commonencodings.Builder) (commonencodings.TypeCodec, error) {
	switch curType {
	case IdlTypeBytes:
		b, err := builder.Int(4)
		if err != nil {
			return nil, err
		}

		return commonencodings.NewSlice(builder.Uint8(), b)
	case IdlTypePublicKey, IdlTypeHash:
		return commonencodings.NewArray(DefaultHashBitLength, builder.Uint8())
	default:
		return nil, fmt.Errorf(unknownIDLFormat, commontypes.ErrInvalidConfig, curType)
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
