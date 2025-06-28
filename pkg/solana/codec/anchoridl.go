package codec

import "github.com/smartcontractkit/chainlink-common/pkg/types/solana"

// Deprecated
type IDL = solana.IDL

// Deprecated
type IdlConstant solana.IdlConstant

// Deprecated
type IdlTypeDefSlice = solana.IdlTypeDefSlice

// Deprecated
type IdlEvent = solana.IdlEvent

// Deprecated
type IdlEventField = solana.IdlEventField

// Deprecated
type IdlInstruction = solana.IdlInstruction

// Deprecated
type IdlAccountItemSlice = solana.IdlAccountItem

// Deprecated
type IdlAccountItem = solana.IdlAccountItem

// Deprecated
type IdlAccount = solana.IdlAccounts

// Deprecated
type IdlAccounts = solana.IdlAccounts

// Deprecated
type IdlField = solana.IdlField

// Deprecated
type PDATypeDef = solana.PDATypeDef

// Deprecated
type PDASeed = solana.PDASeed

// Deprecated
type IdlTypeAsString = solana.IdlTypeAsString

// Deprecated
const (
	IdlTypeBool      = solana.IdlTypeBool
	IdlTypeU8        = solana.IdlTypeU8
	IdlTypeI8        = solana.IdlTypeI8
	IdlTypeU16       = solana.IdlTypeU16
	IdlTypeI16       = solana.IdlTypeI16
	IdlTypeU32       = solana.IdlTypeU32
	IdlTypeI32       = solana.IdlTypeI32
	IdlTypeU64       = solana.IdlTypeU64
	IdlTypeI64       = solana.IdlTypeI64
	IdlTypeU128      = solana.IdlTypeU128
	IdlTypeI128      = solana.IdlTypeI128
	IdlTypeBytes     = solana.IdlTypeBytes
	IdlTypeString    = solana.IdlTypeString
	IdlTypePublicKey = solana.IdlTypePublicKey

	// Custom additions:
	IdlTypeUnixTimestamp = solana.IdlTypeUnixTimestamp
	IdlTypeHash          = solana.IdlTypeHash
	IdlTypeDuration      = solana.IdlTypeDuration
)

// Deprecated
type IdlTypeVec = solana.IdlTypeVec

// Deprecated
type IdlTypeOption = solana.IdlType

// Deprecated
type IdlTypeDefined = solana.IdlTypeDefined

// Deprecated
type IdlTypeArray = solana.IdlTypeArray

// Deprecated
type IdlType = solana.IdlType

// Deprecated
func NewIdlStringType(asString IdlTypeAsString) IdlType {
	return solana.NewIdlStringType(asString)
}

// Deprecated
type IdlTypeDef = solana.IdlTypeDef

// Deprecated
type IdlTypeDefTyKind = solana.IdlTypeDefTyKind

// Deprecated
const (
	IdlTypeDefTyKindStruct = solana.IdlTypeDefTyKindStruct
	IdlTypeDefTyKindEnum   = solana.IdlTypeDefTyKindEnum
	IdlTypeDefTyKindCustom = solana.IdlTypeDefTyKindCustom
)

// Deprecated
type IdlTypeDefTyStruct = solana.IdlTypeDefTyStruct

// Deprecated
type IdlTypeDefTyEnum = solana.IdlTypeDefTyEnum

// Deprecated
var NilIdlTypeDefTy = solana.NilIdlTypeDefTy

// Deprecated
type IdlTypeDefTy = solana.IdlTypeDefTy

// Deprecated
type IdlEnumVariantSlice = solana.IdlEnumVariantSlice

// Deprecated
type IdlTypeDefStruct = solana.IdlTypeDefStruct

// Deprecated
type IdlEnumVariant = solana.IdlEnumVariant

// Deprecated
type IdlEnumFields = solana.IdlEnumFields

// Deprecated
type IdlEnumFieldsNamed solana.IdlEnumFieldsNamed

// Deprecated
type IdlEnumFieldsTuple = solana.IdlEnumFieldsTuple

// Deprecated
type IdlErrorCode = solana.IdlErrorCode
