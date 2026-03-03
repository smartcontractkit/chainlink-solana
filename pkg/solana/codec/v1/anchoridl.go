package codecv1

/*
copied from https://github.com/gagliardetto/anchor-go where the IDL definition is not importable due to being defined
in the `main` package.
*/

import (
	"encoding/json"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/gagliardetto/utilz"
)

// https://github.com/project-serum/anchor/blob/97e9e03fb041b8b888a9876a7c0676d9bb4736f3/ts/src/idl.ts
type IDL struct {
	Version      string           `json:"version"`
	Name         string           `json:"name"`
	Instructions []IdlInstruction `json:"instructions"`
	Accounts     IdlTypeDefSlice  `json:"accounts,omitempty"`
	Types        IdlTypeDefSlice  `json:"types,omitempty"`
	Events       []IdlEvent       `json:"events,omitempty"`
	Errors       []IdlErrorCode   `json:"errors,omitempty"`
	Constants    []IdlConstant    `json:"constants,omitempty"`
}

type IdlConstant struct {
	Name  string
	Type  IdlType
	Value string
}

type IdlTypeDefSlice []IdlTypeDef

func (named IdlTypeDefSlice) GetByName(name string) *IdlTypeDef {
	for i := range named {
		v := named[i]
		if v.Name == name {
			return &v
		}
	}
	return nil
}

type IdlEvent struct {
	Name   string          `json:"name"`
	Fields []IdlEventField `json:"fields"`
}

type IdlEventField struct {
	Name  string  `json:"name"`
	Type  IdlType `json:"type"`
	Index bool    `json:"index"`
}

type IdlInstruction struct {
	Name     string              `json:"name"`
	Docs     []string            `json:"docs"` // @custom
	Accounts IdlAccountItemSlice `json:"accounts"`
	Args     []IdlField          `json:"args"`
}

type IdlAccountItemSlice []IdlAccountItem

func (slice IdlAccountItemSlice) NumAccounts() (count int) {
	for _, item := range slice {
		if item.IdlAccount != nil {
			count++
		}

		if item.IdlAccounts != nil {
			count += item.IdlAccounts.Accounts.NumAccounts()
		}
	}

	return count
}

// type IdlAccountItem = IdlAccount | IdlAccounts;
type IdlAccountItem struct {
	IdlAccount  *IdlAccount
	IdlAccounts *IdlAccounts
}

func (env *IdlAccountItem) UnmarshalJSON(data []byte) error {
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if temp == nil {
		return fmt.Errorf("envelope is nil: %v", env)
	}

	switch v := temp.(type) {
	case map[string]interface{}:
		if len(v) == 0 {
			return nil
		}

		_, hasAccounts := v["accounts"]
		_, hasIsMut := v["isMut"]

		if hasAccounts == hasIsMut {
			return fmt.Errorf("invalid idl structure: expected exactly one of 'accounts' or 'isMut'")
		}

		if hasAccounts {
			return utilz.TranscodeJSON(temp, &env.IdlAccounts)
		}

		return utilz.TranscodeJSON(temp, &env.IdlAccount)
	default:
		return fmt.Errorf("unknown kind: %s", spew.Sdump(temp))
	}
}

func (env IdlAccountItem) MarshalJSON() ([]byte, error) {
	if (env.IdlAccount == nil) == (env.IdlAccounts == nil) {
		return nil, fmt.Errorf("invalid structure: expected either IdlAccount or IdlAccounts to be defined")
	}

	visited := make(map[*IdlAccounts]struct{})
	if err := checkForIdlAccountsCycle(env.IdlAccounts, visited); err != nil {
		return nil, err
	}

	var result interface{}
	if env.IdlAccounts != nil {
		result = map[string]interface{}{
			"accounts": env.IdlAccounts,
		}
	} else {
		result = env.IdlAccount
	}

	return json.Marshal(result)
}

func checkForIdlAccountsCycle(acc *IdlAccounts, visited map[*IdlAccounts]struct{}) error {
	if acc == nil {
		return nil
	}

	if _, exists := visited[acc]; exists {
		return fmt.Errorf("cycle detected in IdlAccounts named %q", acc.Name)
	}
	visited[acc] = struct{}{}

	for _, item := range acc.Accounts {
		if (item.IdlAccount == nil) == (item.IdlAccounts == nil) {
			return fmt.Errorf("invalid nested structure: expected either IdlAccount or IdlAccounts to be defined")
		}
		if item.IdlAccounts != nil {
			if err := checkForIdlAccountsCycle(item.IdlAccounts, visited); err != nil {
				return err
			}
		}
	}
	return nil
}

type IdlAccount struct {
	Docs     []string `json:"docs"` // @custom
	Name     string   `json:"name"`
	IsMut    bool     `json:"isMut"`
	IsSigner bool     `json:"isSigner"`
	Optional bool     `json:"optional"` // @custom
}

// A nested/recursive version of IdlAccount.
type IdlAccounts struct {
	Name     string              `json:"name"`
	Docs     []string            `json:"docs"` // @custom
	Accounts IdlAccountItemSlice `json:"accounts"`
}

type IdlField struct {
	Name string   `json:"name"`
	Docs []string `json:"docs"` // @custom
	Type IdlType  `json:"type"`
}

// PDA is a struct that does not correlate to an official IDL type
// It is needed to encode seeds to calculate the address for PDA account reads
type PDATypeDef struct {
	Prefix []byte    `json:"prefix,omitempty"`
	Seeds  []PDASeed `json:"seeds,omitempty"`
}

type PDASeed struct {
	Name string  `json:"name"`
	Type IdlType `json:"type"`
}

type IdlTypeAsString string

const (
	IdlTypeBool      IdlTypeAsString = "bool"
	IdlTypeU8        IdlTypeAsString = "u8"
	IdlTypeI8        IdlTypeAsString = "i8"
	IdlTypeU16       IdlTypeAsString = "u16"
	IdlTypeI16       IdlTypeAsString = "i16"
	IdlTypeU32       IdlTypeAsString = "u32"
	IdlTypeI32       IdlTypeAsString = "i32"
	IdlTypeU64       IdlTypeAsString = "u64"
	IdlTypeI64       IdlTypeAsString = "i64"
	IdlTypeU128      IdlTypeAsString = "u128"
	IdlTypeI128      IdlTypeAsString = "i128"
	IdlTypeBytes     IdlTypeAsString = "bytes"
	IdlTypeString    IdlTypeAsString = "string"
	IdlTypePublicKey IdlTypeAsString = "publicKey"

	// Custom additions:
	IdlTypeUnixTimestamp IdlTypeAsString = "unixTimestamp"
	IdlTypeHash          IdlTypeAsString = "hash"
	IdlTypeDuration      IdlTypeAsString = "duration"
)

type IdlTypeVec struct {
	Vec IdlType `json:"vec"`
}

type IdlTypeOption struct {
	Option IdlType `json:"option"`
}

// User defined type.
type IdlTypeDefined struct {
	Defined string `json:"defined"`
}

// Wrapper type:
type IdlTypeArray struct {
	Thing IdlType
	Num   int
}

func (env IdlType) MarshalJSON() ([]byte, error) {
	var result interface{}
	switch {
	case env.IsString():
		result = env.GetString()
	case env.IsIdlTypeVec():
		result = env.GetIdlTypeVec()
	case env.IsIdlTypeOption():
		result = env.GetIdlTypeOption()
	case env.IsIdlTypeDefined():
		result = env.GetIdlTypeDefined()
	case env.IsArray():
		array := env.GetArray()
		result = map[string]interface{}{
			"array": []interface{}{array.Thing, array.Num},
		}
	default:
		return nil, fmt.Errorf("nil envelope is not supported in IdlType")
	}

	return json.Marshal(result)
}

func (env *IdlType) UnmarshalJSON(data []byte) error {
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if temp == nil {
		return fmt.Errorf("envelope is nil: %v", env)
	}

	switch v := temp.(type) {
	case string:
		env.AsString = IdlTypeAsString(v)
	case map[string]interface{}:
		if len(v) == 0 {
			return nil
		}

		var typeFound bool
		if _, ok := v["vec"]; ok {
			var target IdlTypeVec
			if err := utilz.TranscodeJSON(temp, &target); err != nil {
				return err
			}
			typeFound = true
			env.AsIdlTypeVec = &target
		}
		if _, ok := v["option"]; ok {
			if typeFound {
				return fmt.Errorf("multiple types found for IdlType: %s", spew.Sdump(temp))
			}
			var target IdlTypeOption
			if err := utilz.TranscodeJSON(temp, &target); err != nil {
				return err
			}
			typeFound = true
			env.asIdlTypeOption = &target
		}
		if _, ok := v["defined"]; ok {
			if typeFound {
				return fmt.Errorf("multiple types found for IdlType: %s", spew.Sdump(temp))
			}
			var target IdlTypeDefined
			if err := utilz.TranscodeJSON(temp, &target); err != nil {
				return err
			}
			typeFound = true
			env.AsIdlTypeDefined = &target
		}
		if got, ok := v["array"]; ok {
			if typeFound {
				return fmt.Errorf("multiple types found for IdlType: %s", spew.Sdump(temp))
			}
			arrVal, ok := got.([]interface{})
			if !ok {
				return fmt.Errorf("array is not in expected format: %s", spew.Sdump(got))
			}
			if len(arrVal) != 2 {
				return fmt.Errorf("array is not of expected length: %s", spew.Sdump(got))
			}
			var target IdlTypeArray
			if err := utilz.TranscodeJSON(arrVal[0], &target.Thing); err != nil {
				return err
			}
			num, ok := arrVal[1].(float64)
			if !ok {
				return fmt.Errorf("value is unexpected type: %T, expected float64", arrVal[1])
			}
			target.Num = int(num)
			env.AsIdlTypeArray = &target
		}
	default:
		return fmt.Errorf("Unknown kind: %s", spew.Sdump(temp))
	}

	return nil
}

// Wrapper type:
type IdlType struct {
	AsString         IdlTypeAsString
	AsIdlTypeVec     *IdlTypeVec
	asIdlTypeOption  *IdlTypeOption
	AsIdlTypeDefined *IdlTypeDefined
	AsIdlTypeArray   *IdlTypeArray
}

func NewIdlStringType(asString IdlTypeAsString) IdlType {
	return IdlType{
		AsString: asString,
	}
}

func (env *IdlType) IsString() bool {
	return env.AsString != ""
}
func (env *IdlType) IsIdlTypeVec() bool {
	return env.AsIdlTypeVec != nil
}
func (env *IdlType) IsIdlTypeOption() bool {
	return env.asIdlTypeOption != nil
}
func (env *IdlType) IsIdlTypeDefined() bool {
	return env.AsIdlTypeDefined != nil
}
func (env *IdlType) IsArray() bool {
	return env.AsIdlTypeArray != nil
}

// Getters:
func (env *IdlType) GetString() IdlTypeAsString {
	return env.AsString
}
func (env *IdlType) GetIdlTypeVec() *IdlTypeVec {
	return env.AsIdlTypeVec
}
func (env *IdlType) GetIdlTypeOption() *IdlTypeOption {
	return env.asIdlTypeOption
}
func (env *IdlType) GetIdlTypeDefined() *IdlTypeDefined {
	return env.AsIdlTypeDefined
}
func (env *IdlType) GetArray() *IdlTypeArray {
	return env.AsIdlTypeArray
}

type IdlTypeDef struct {
	Name string       `json:"name"`
	Type IdlTypeDefTy `json:"type"`
}

type IdlTypeDefTyKind string

const (
	IdlTypeDefTyKindStruct IdlTypeDefTyKind = "struct"
	IdlTypeDefTyKindEnum   IdlTypeDefTyKind = "enum"
	IdlTypeDefTyKindCustom IdlTypeDefTyKind = "custom"
)

type IdlTypeDefTyStruct struct {
	Kind IdlTypeDefTyKind `json:"kind"` // == "struct"

	Fields *IdlTypeDefStruct `json:"fields,omitempty"`
}

type IdlTypeDefTyEnum struct {
	Kind IdlTypeDefTyKind `json:"kind"` // == "enum"

	Variants IdlEnumVariantSlice `json:"variants,omitempty"`
}

var NilIdlTypeDefTy = IdlTypeDef{Type: IdlTypeDefTy{
	Kind:   "struct",
	Fields: &IdlTypeDefStruct{},
}}

type IdlTypeDefTy struct {
	Kind IdlTypeDefTyKind `json:"kind"`

	Fields   *IdlTypeDefStruct   `json:"fields,omitempty"`
	Variants IdlEnumVariantSlice `json:"variants,omitempty"`
	Codec    string              `json:"codec,omitempty"`
}

type IdlEnumVariantSlice []IdlEnumVariant

func (slice IdlEnumVariantSlice) IsAllUint8() bool {
	for _, elem := range slice {
		if !elem.IsUint8() {
			return false
		}
	}
	return true
}

func (slice IdlEnumVariantSlice) IsSimpleEnum() bool {
	return slice.IsAllUint8()
}

type IdlTypeDefStruct = []IdlField

type IdlEnumVariant struct {
	Name   string         `json:"name"`
	Docs   []string       `json:"docs"` // @custom
	Fields *IdlEnumFields `json:"fields,omitempty"`
}

func (variant *IdlEnumVariant) IsUint8() bool {
	// it's a simple uint8 if there is no fields data
	return variant.Fields == nil
}

// type IdlEnumFields = IdlEnumFieldsNamed | IdlEnumFieldsTuple;
type IdlEnumFields struct {
	IdlEnumFieldsNamed *IdlEnumFieldsNamed
	IdlEnumFieldsTuple *IdlEnumFieldsTuple
}

type IdlEnumFieldsNamed []IdlField

type IdlEnumFieldsTuple []IdlType

// TODO: verify with examples
func (env *IdlEnumFields) UnmarshalJSON(data []byte) error {
	var temp any
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if temp == nil {
		return fmt.Errorf("envelope is nil: %v", env)
	}

	switch v := temp.(type) {
	case []any:
		if len(v) == 0 {
			return nil
		}

		firstItem := v[0]

		if _, ok := firstItem.(map[string]any)["name"]; ok {
			// TODO:
			// If has `name` field, then it's most likely a IdlEnumFieldsNamed.
			return utilz.TranscodeJSON(temp, &env.IdlEnumFieldsNamed)
		}
		return utilz.TranscodeJSON(temp, &env.IdlEnumFieldsTuple)
	case map[string]any:
		// Only one or the other field is set. Returning early is safe
		if named, ok := v["IdlEnumFieldsNamed"]; ok {
			return utilz.TranscodeJSON(named, &env.IdlEnumFieldsNamed)
		}
		if tuple, ok := v["IdlEnumFieldsTuple"]; ok {
			return utilz.TranscodeJSON(tuple, &env.IdlEnumFieldsTuple)
		}
		return fmt.Errorf("Unknown type: %s", spew.Sdump(v))
	default:
		return fmt.Errorf("Unknown kind: %s", spew.Sdump(temp))
	}
}

type IdlErrorCode struct {
	Code int    `json:"code"`
	Name string `json:"name"`
	Msg  string `json:"msg,omitempty"`
}
