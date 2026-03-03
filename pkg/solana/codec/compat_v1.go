package codec

import (
	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

// Backward-compatible aliases for the old pkg/solana/codec public API.
// The underlying implementation now lives in pkg/solana/codec/v1 and pkg/solana/codec/common.

type ChainConfigType = solcommoncodec.ChainConfigType

const (
	ChainConfigTypeAccountDef     = solcommoncodec.ChainConfigTypeAccountDef
	ChainConfigTypeInstructionDef = solcommoncodec.ChainConfigTypeInstructionDef
	ChainConfigTypeEventDef       = solcommoncodec.ChainConfigTypeEventDef
)

type Config = solcommoncodec.Config
type ChainConfig = solcommoncodec.ChainConfig
type Entry = solcommoncodec.Entry
type ParsedTypes = solcommoncodec.ParsedTypes

var DecoderHooks = solcommoncodec.DecoderHooks

func WrapItemType(forEncoding bool, contractName, itemType string) string {
	return solcommoncodec.WrapItemType(forEncoding, contractName, itemType)
}

func AddEntries(defs map[string]Entry, modByTypeName map[string]map[string]commoncodec.Modifier) error {
	return solcommoncodec.AddEntries(defs, modByTypeName)
}

func EntryAsModifierRemoteCodec(entry Entry, itemType string) (commontypes.RemoteCodec, error) {
	return solcommoncodec.EntryAsModifierRemoteCodec(entry, itemType)
}

type IDL = codecv1.IDL
type IdlConstant = codecv1.IdlConstant
type IdlTypeDefSlice = codecv1.IdlTypeDefSlice
type IdlEvent = codecv1.IdlEvent
type IdlEventField = codecv1.IdlEventField
type IdlInstruction = codecv1.IdlInstruction
type IdlAccountItemSlice = codecv1.IdlAccountItemSlice
type IdlAccountItem = codecv1.IdlAccountItem
type IdlAccount = codecv1.IdlAccount
type IdlAccounts = codecv1.IdlAccounts
type IdlField = codecv1.IdlField
type PDATypeDef = codecv1.PDATypeDef
type PDASeed = codecv1.PDASeed
type IdlTypeAsString = codecv1.IdlTypeAsString
type IdlTypeVec = codecv1.IdlTypeVec
type IdlTypeOption = codecv1.IdlTypeOption
type IdlTypeDefined = codecv1.IdlTypeDefined
type IdlTypeArray = codecv1.IdlTypeArray
type IdlType = codecv1.IdlType
type IdlTypeDef = codecv1.IdlTypeDef
type IdlTypeDefTyKind = codecv1.IdlTypeDefTyKind
type IdlTypeDefTyStruct = codecv1.IdlTypeDefTyStruct
type IdlTypeDefTyEnum = codecv1.IdlTypeDefTyEnum
type IdlTypeDefTy = codecv1.IdlTypeDefTy
type IdlEnumVariantSlice = codecv1.IdlEnumVariantSlice
type IdlTypeDefStruct = codecv1.IdlTypeDefStruct
type IdlEnumVariant = codecv1.IdlEnumVariant
type IdlEnumFields = codecv1.IdlEnumFields
type IdlEnumFieldsNamed = codecv1.IdlEnumFieldsNamed
type IdlEnumFieldsTuple = codecv1.IdlEnumFieldsTuple
type IdlErrorCode = codecv1.IdlErrorCode

const (
	IdlTypeDefTyKindStruct = codecv1.IdlTypeDefTyKindStruct
	IdlTypeDefTyKindEnum   = codecv1.IdlTypeDefTyKindEnum
	IdlTypeDefTyKindCustom = codecv1.IdlTypeDefTyKindCustom
)

const (
	IdlTypeBool          = codecv1.IdlTypeBool
	IdlTypeU8            = codecv1.IdlTypeU8
	IdlTypeI8            = codecv1.IdlTypeI8
	IdlTypeU16           = codecv1.IdlTypeU16
	IdlTypeI16           = codecv1.IdlTypeI16
	IdlTypeU32           = codecv1.IdlTypeU32
	IdlTypeI32           = codecv1.IdlTypeI32
	IdlTypeU64           = codecv1.IdlTypeU64
	IdlTypeI64           = codecv1.IdlTypeI64
	IdlTypeU128          = codecv1.IdlTypeU128
	IdlTypeI128          = codecv1.IdlTypeI128
	IdlTypeBytes         = codecv1.IdlTypeBytes
	IdlTypeString        = codecv1.IdlTypeString
	IdlTypePublicKey     = codecv1.IdlTypePublicKey
	IdlTypeUnixTimestamp = codecv1.IdlTypeUnixTimestamp
	IdlTypeHash          = codecv1.IdlTypeHash
	IdlTypeDuration      = codecv1.IdlTypeDuration
)

const DefaultHashBitLength = codecv1.DefaultHashBitLength

var NilIdlTypeDefTy = codecv1.NilIdlTypeDefTy

func NewIdlStringType(asString IdlTypeAsString) IdlType {
	return codecv1.NewIdlStringType(asString)
}

type AccountIDLTypes = codecv1.AccountIDLTypes
type InstructionArgsIDLTypes = codecv1.InstructionArgsIDLTypes
type EventIDLTypes = codecv1.EventIDLTypes

func NewAccountEntry(offchainName string, idlTypes AccountIDLTypes, includeDiscriminator bool, mod commoncodec.Modifier, builder encodings.Builder) (Entry, error) {
	return codecv1.NewAccountEntry(offchainName, idlTypes, includeDiscriminator, mod, builder)
}

func NewPDAEntry(offchainName string, pdaTypeDef PDATypeDef, mod commoncodec.Modifier, builder encodings.Builder) (Entry, error) {
	return codecv1.NewPDAEntry(offchainName, pdaTypeDef, mod, builder)
}

func NewInstructionArgsEntry(offChainName string, idlTypes InstructionArgsIDLTypes, mod commoncodec.Modifier, builder encodings.Builder) (Entry, error) {
	return codecv1.NewInstructionArgsEntry(offChainName, idlTypes, mod, builder)
}

// func NewEventArgsEntryWrapper(offChainName string, contractIdl string, includeDiscriminator bool, mod commoncodec.Modifier, builder encodings.Builder) (Entry, error) {
// 	return codecv1.NewEventArgsEntryWrapper(offChainName, contractIdl, includeDiscriminator, mod, builder)
// }

func NewEventArgsEntry(offChainName string, idlTypes EventIDLTypes, includeDiscriminator bool, mod commoncodec.Modifier, builder encodings.Builder) (Entry, error) {
	return codecv1.NewEventArgsEntry(offChainName, idlTypes, includeDiscriminator, mod, builder)
}

func NewOnRampAddress(builder encodings.Builder) encodings.TypeCodec {
	return codecv1.NewOnRampAddress(builder)
}

func NewCodec(conf Config) (commontypes.RemoteCodec, error) {
	return codecv1.NewCodec(conf)
}

func CreateCodecEntry(idlDefinition interface{}, offChainName string, idl IDL, mod commoncodec.Modifier) (entry Entry, err error) {
	return codecv1.CreateCodecEntry(idlDefinition, offChainName, idl, mod)
}

// func CreateCodecEntryWrapper(cfgType ChainConfigType, mod commoncodec.Modifier, onChainName, offChainName, idlString string) (entry Entry, err error) {
// 	return codecv1.CreateCodecEntryWrapper(cfgType, mod, onChainName, offChainName, idlString)
// }

func FindDefinitionFromIDL(cfgType ChainConfigType, chainSpecificName string, idl IDL) (interface{}, error) {
	return codecv1.FindDefinitionFromIDL(cfgType, chainSpecificName, idl)
}

func ExtractEventIDL(eventName string, idl IDL) (IdlEvent, error) {
	return codecv1.ExtractEventIDL(eventName, idl)
}

func NewIDLAccountCodec(idl IDL, builder encodings.Builder) (commontypes.RemoteCodec, error) {
	return codecv1.NewIDLAccountCodec(idl, builder)
}

func NewNamedModifierCodec(original commontypes.RemoteCodec, itemType string, modifier commoncodec.Modifier) (commontypes.RemoteCodec, error) {
	return codecv1.NewNamedModifierCodec(original, itemType, modifier)
}

func NewIDLDefinedTypesCodec(idl IDL, builder encodings.Builder) (commontypes.RemoteCodec, error) {
	return codecv1.NewIDLDefinedTypesCodec(idl, builder)
}
