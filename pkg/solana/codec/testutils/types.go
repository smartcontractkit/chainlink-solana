package testutils

import (
	_ "embed"
	"fmt"
	"math/big"
	"time"

	agbinary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

var (
	TestStructWithNestedStruct     = "StructWithNestedStruct"
	TestStructWithNestedStructType = "StructWithNestedStructType"
	DefaultStringRef               = "test string"
	DefaultTestStruct              = StructWithNestedStruct{
		Value: 80,
		InnerStruct: ObjectRef1{
			Prop1: 10,
			Prop2: "some_val",
			Prop3: new(big.Int).SetUint64(42),
			Prop4: 42,
			Prop5: 42,
			Prop6: true,
		},
		BasicNestedArray: [][]uint32{{5, 6, 7}, {0, 0, 0}, {0, 0, 0}},
		Option:           &DefaultStringRef,
		DefinedArray: []ObjectRef2{
			{
				Prop1: 42,
				Prop2: new(big.Int).SetInt64(42),
				Prop3: 43,
				Prop4: 44,
				Prop5: 45,
				Prop6: []byte{},
			},
			{
				Prop1: 46,
				Prop2: new(big.Int).SetInt64(46),
				Prop3: 47,
				Prop4: 48,
				Prop5: 49,
				Prop6: []byte{},
			},
		},
		BasicVector: []string{"some string", "another string"},
		TimeVal:     683_100_000,
		DurationVal: 42 * time.Second,
		PublicKey:   solana.NewWallet().PublicKey(),
		EnumVal:     0,
	}
	TestItemWithConfigExtraType = "TestItemWithConfigExtra"
	TestEventItem               = "TestEventItem"
)

type StructWithNestedStruct struct {
	Value            uint8
	InnerStruct      ObjectRef1
	BasicNestedArray [][]uint32
	Option           *string
	DefinedArray     []ObjectRef2
	BasicVector      []string
	TimeVal          int64
	DurationVal      time.Duration
	PublicKey        solana.PublicKey
	EnumVal          uint8
}

type ObjectRef1 struct {
	Prop1 int8
	Prop2 string
	Prop3 *big.Int
	Prop4 uint16
	Prop5 uint64
	Prop6 bool
}

type ObjectRef2 struct {
	Prop1 uint32
	Prop2 *big.Int
	Prop3 int16
	Prop4 int32
	Prop5 int64
	Prop6 []byte
}

//go:embed testIDL.json
var JSONIDLWithAllTypes string

//go:embed circularDepIDL.json
var CircularDepIDL string

//go:embed itemIDL.json
var itemTypeJSONIDL string

//go:embed eventItemTypeIDL.json
var eventItemTypeJSONIDL string

//go:embed itemSliceTypeIDL.json
var itemSliceTypeJSONIDL string

//go:embed itemArray1TypeIDL.json
var itemArray1TypeJSONIDL string

//go:embed itemArray2TypeIDL.json
var itemArray2TypeJSONIDL string

//go:embed nilTypeIDL.json
var nilTypeJSONIDL string

type CodecDef struct {
	IDL         string
	IDLTypeName string
	ItemType    codec.ChainConfigType
}

// CodecDefs key is codec offchain type name
var CodecDefs = map[string]CodecDef{
	interfacetests.TestItemType: {
		IDL:         itemTypeJSONIDL,
		IDLTypeName: interfacetests.TestItemType,
		ItemType:    codec.ChainConfigTypeAccountDef,
	},
	interfacetests.TestItemSliceType: {
		IDL:         itemSliceTypeJSONIDL,
		IDLTypeName: interfacetests.TestItemSliceType,
		ItemType:    codec.ChainConfigTypeInstructionDef,
	},
	interfacetests.TestItemArray1Type: {
		IDL:         itemArray1TypeJSONIDL,
		IDLTypeName: interfacetests.TestItemArray1Type,
		ItemType:    codec.ChainConfigTypeInstructionDef,
	},
	interfacetests.TestItemArray2Type: {
		IDL:         itemArray2TypeJSONIDL,
		IDLTypeName: interfacetests.TestItemArray2Type,
		ItemType:    codec.ChainConfigTypeInstructionDef,
	},
	TestItemWithConfigExtraType: {
		IDL:         itemTypeJSONIDL,
		IDLTypeName: interfacetests.TestItemType,
		ItemType:    codec.ChainConfigTypeAccountDef,
	},
	interfacetests.NilType: {
		IDL:         nilTypeJSONIDL,
		IDLTypeName: interfacetests.NilType,
		ItemType:    codec.ChainConfigTypeAccountDef,
	},
	TestEventItem: {
		IDL:         eventItemTypeJSONIDL,
		IDLTypeName: interfacetests.TestItemType,
		ItemType:    codec.ChainConfigTypeEventDef,
	},
}

type TestItemAsAccount struct {
	Field               int32
	OracleID            uint8
	OracleIDs           [32]uint8
	AccountStruct       AccountStruct
	Accounts            []solana.PublicKey
	DifferentField      string
	BigField            agbinary.Int128
	NestedDynamicStruct NestedDynamic
	NestedStaticStruct  NestedStatic
}

var TestItemAsAccountDiscriminator = [8]byte{148, 105, 105, 155, 26, 167, 212, 149}

func (obj TestItemAsAccount) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Write account discriminator:
	err = encoder.WriteBytes(TestItemAsAccountDiscriminator[:], false)
	if err != nil {
		return err
	}
	// Serialize `Field` param:
	err = encoder.Encode(obj.Field)
	if err != nil {
		return err
	}
	// Serialize `OracleID` param:
	err = encoder.Encode(obj.OracleID)
	if err != nil {
		return err
	}
	// Serialize `OracleIDs` param:
	err = encoder.Encode(obj.OracleIDs)
	if err != nil {
		return err
	}
	// Serialize `AccountStruct` param:
	err = encoder.Encode(obj.AccountStruct)
	if err != nil {
		return err
	}
	// Serialize `Accounts` param:
	err = encoder.Encode(obj.Accounts)
	if err != nil {
		return err
	}
	// Serialize `DifferentField` param:
	err = encoder.Encode(obj.DifferentField)
	if err != nil {
		return err
	}
	// Serialize `BigField` param:
	err = encoder.Encode(obj.BigField)
	if err != nil {
		return err
	}
	// Serialize `NestedDynamicStruct` param:
	err = encoder.Encode(obj.NestedDynamicStruct)
	if err != nil {
		return err
	}
	// Serialize `NestedStaticStruct` param:
	err = encoder.Encode(obj.NestedStaticStruct)
	if err != nil {
		return err
	}
	return nil
}

func (obj *TestItemAsAccount) UnmarshalWithDecoder(decoder *agbinary.Decoder) error {
	// Read and check account discriminator:
	{
		discriminator, err := decoder.ReadTypeID()
		if err != nil {
			return err
		}
		if !discriminator.Equal(TestItemAsAccountDiscriminator[:]) {
			return fmt.Errorf(
				"wrong discriminator: wanted %s, got %s",
				"[148 105 105 155 26 167 212 149]",
				fmt.Sprint(discriminator[:]))
		}
	}
	// Deserialize `Field`:
	err := decoder.Decode(&obj.Field)
	if err != nil {
		return err
	}
	// Deserialize `OracleID`:
	err = decoder.Decode(&obj.OracleID)
	if err != nil {
		return err
	}
	// Deserialize `OracleIDs`:
	err = decoder.Decode(&obj.OracleIDs)
	if err != nil {
		return err
	}
	// Deserialize `AccountStruct`:
	err = decoder.Decode(&obj.AccountStruct)
	if err != nil {
		return err
	}
	// Deserialize `Accounts`:
	err = decoder.Decode(&obj.Accounts)
	if err != nil {
		return err
	}
	// Deserialize `DifferentField`:
	err = decoder.Decode(&obj.DifferentField)
	if err != nil {
		return err
	}
	// Deserialize `BigField`:
	err = decoder.Decode(&obj.BigField)
	if err != nil {
		return err
	}
	// Deserialize `NestedDynamicStruct`:
	err = decoder.Decode(&obj.NestedDynamicStruct)
	if err != nil {
		return err
	}
	// Deserialize `NestedStaticStruct`:
	err = decoder.Decode(&obj.NestedStaticStruct)
	if err != nil {
		return err
	}
	return nil
}

type TestItemAsEvent struct {
	Field               int32
	OracleID            uint8
	OracleIDs           [32]uint8
	AccountStruct       AccountStruct
	Accounts            []solana.PublicKey
	DifferentField      string
	BigField            agbinary.Int128
	NestedDynamicStruct NestedDynamic
	NestedStaticStruct  NestedStatic
}

var TestItemAsEventDiscriminator = [8]byte{119, 183, 160, 247, 84, 104, 222, 251}

func (obj TestItemAsEvent) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Write event discriminator:
	err = encoder.WriteBytes(TestItemAsEventDiscriminator[:], false)
	if err != nil {
		return err
	}
	// Serialize `Field` param:
	err = encoder.Encode(obj.Field)
	if err != nil {
		return err
	}
	// Serialize `OracleID` param:
	err = encoder.Encode(obj.OracleID)
	if err != nil {
		return err
	}
	// Serialize `OracleIDs` param:
	err = encoder.Encode(obj.OracleIDs)
	if err != nil {
		return err
	}
	// Serialize `AccountStruct` param:
	err = encoder.Encode(obj.AccountStruct)
	if err != nil {
		return err
	}
	// Serialize `Accounts` param:
	err = encoder.Encode(obj.Accounts)
	if err != nil {
		return err
	}
	// Serialize `DifferentField` param:
	err = encoder.Encode(obj.DifferentField)
	if err != nil {
		return err
	}
	// Serialize `BigField` param:
	err = encoder.Encode(obj.BigField)
	if err != nil {
		return err
	}
	// Serialize `NestedDynamicStruct` param:
	err = encoder.Encode(obj.NestedDynamicStruct)
	if err != nil {
		return err
	}
	// Serialize `NestedStaticStruct` param:
	err = encoder.Encode(obj.NestedStaticStruct)
	if err != nil {
		return err
	}
	return nil
}

func (obj *TestItemAsEvent) UnmarshalWithDecoder(decoder *agbinary.Decoder) error {
	// Read and check account discriminator:
	{
		discriminator, err := decoder.ReadTypeID()
		if err != nil {
			return err
		}
		if !discriminator.Equal(TestItemAsEventDiscriminator[:]) {
			return fmt.Errorf(
				"wrong discriminator: wanted %s, got %s",
				"[119, 183, 160, 247, 84, 104, 222, 251]",
				fmt.Sprint(discriminator[:]))
		}
	}
	// Deserialize `Field`:
	err := decoder.Decode(&obj.Field)
	if err != nil {
		return err
	}
	// Deserialize `OracleID`:
	err = decoder.Decode(&obj.OracleID)
	if err != nil {
		return err
	}
	// Deserialize `OracleIDs`:
	err = decoder.Decode(&obj.OracleIDs)
	if err != nil {
		return err
	}
	// Deserialize `AccountStruct`:
	err = decoder.Decode(&obj.AccountStruct)
	if err != nil {
		return err
	}
	// Deserialize `Accounts`:
	err = decoder.Decode(&obj.Accounts)
	if err != nil {
		return err
	}
	// Deserialize `DifferentField`:
	err = decoder.Decode(&obj.DifferentField)
	if err != nil {
		return err
	}
	// Deserialize `BigField`:
	err = decoder.Decode(&obj.BigField)
	if err != nil {
		return err
	}
	// Deserialize `NestedDynamicStruct`:
	err = decoder.Decode(&obj.NestedDynamicStruct)
	if err != nil {
		return err
	}
	// Deserialize `NestedStaticStruct`:
	err = decoder.Decode(&obj.NestedStaticStruct)
	if err != nil {
		return err
	}
	return nil
}

type TestItemAsArgs struct {
	Field               int32
	OracleID            uint8
	OracleIDs           [32]uint8
	AccountStruct       AccountStruct
	Accounts            []solana.PublicKey
	DifferentField      string
	BigField            agbinary.Int128
	NestedDynamicStruct NestedDynamic
	NestedStaticStruct  NestedStatic
}

func (obj TestItemAsArgs) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Serialize `Field` param:
	err = encoder.Encode(obj.Field)
	if err != nil {
		return err
	}
	// Serialize `OracleID` param:
	err = encoder.Encode(obj.OracleID)
	if err != nil {
		return err
	}
	// Serialize `OracleIDs` param:
	err = encoder.Encode(obj.OracleIDs)
	if err != nil {
		return err
	}
	// Serialize `AccountStruct` param:
	err = encoder.Encode(obj.AccountStruct)
	if err != nil {
		return err
	}
	// Serialize `Accounts` param:
	err = encoder.Encode(obj.Accounts)
	if err != nil {
		return err
	}
	// Serialize `DifferentField` param:
	err = encoder.Encode(obj.DifferentField)
	if err != nil {
		return err
	}
	// Serialize `BigField` param:
	err = encoder.Encode(obj.BigField)
	if err != nil {
		return err
	}
	// Serialize `NestedDynamicStruct` param:
	err = encoder.Encode(obj.NestedDynamicStruct)
	if err != nil {
		return err
	}
	// Serialize `NestedStaticStruct` param:
	err = encoder.Encode(obj.NestedStaticStruct)
	if err != nil {
		return err
	}
	return nil
}

func (obj *TestItemAsArgs) UnmarshalWithDecoder(decoder *agbinary.Decoder) (err error) {
	// Deserialize `Field`:
	err = decoder.Decode(&obj.Field)
	if err != nil {
		return err
	}
	// Deserialize `OracleID`:
	err = decoder.Decode(&obj.OracleID)
	if err != nil {
		return err
	}
	// Deserialize `OracleIDs`:
	err = decoder.Decode(&obj.OracleIDs)
	if err != nil {
		return err
	}
	// Deserialize `AccountStruct`:
	err = decoder.Decode(&obj.AccountStruct)
	if err != nil {
		return err
	}
	// Deserialize `Accounts`:
	err = decoder.Decode(&obj.Accounts)
	if err != nil {
		return err
	}
	// Deserialize `DifferentField`:
	err = decoder.Decode(&obj.DifferentField)
	if err != nil {
		return err
	}
	// Deserialize `BigField`:
	err = decoder.Decode(&obj.BigField)
	if err != nil {
		return err
	}
	// Deserialize `NestedDynamicStruct`:
	err = decoder.Decode(&obj.NestedDynamicStruct)
	if err != nil {
		return err
	}
	// Deserialize `NestedStaticStruct`:
	err = decoder.Decode(&obj.NestedStaticStruct)
	if err != nil {
		return err
	}
	return nil
}

type AccountStruct struct {
	Account    solana.PublicKey
	AccountStr solana.PublicKey
}

func (obj AccountStruct) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Serialize `Account` param:
	err = encoder.Encode(obj.Account)
	if err != nil {
		return err
	}
	// Serialize `AccountStr` param:
	err = encoder.Encode(obj.AccountStr)
	if err != nil {
		return err
	}
	return nil
}

func (obj *AccountStruct) UnmarshalWithDecoder(decoder *agbinary.Decoder) (err error) {
	// Deserialize `Account`:
	err = decoder.Decode(&obj.Account)
	if err != nil {
		return err
	}
	// Deserialize `AccountStr`:
	err = decoder.Decode(&obj.AccountStr)
	if err != nil {
		return err
	}
	return nil
}

type InnerDynamic struct {
	IntVal int64
	S      string
}

func (obj InnerDynamic) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Serialize `IntVal` param:
	err = encoder.Encode(obj.IntVal)
	if err != nil {
		return err
	}
	// Serialize `S` param:
	err = encoder.Encode(obj.S)
	if err != nil {
		return err
	}
	return nil
}

func (obj *InnerDynamic) UnmarshalWithDecoder(decoder *agbinary.Decoder) (err error) {
	// Deserialize `IntVal`:
	err = decoder.Decode(&obj.IntVal)
	if err != nil {
		return err
	}
	// Deserialize `S`:
	err = decoder.Decode(&obj.S)
	if err != nil {
		return err
	}
	return nil
}

type NestedDynamic struct {
	FixedBytes [2]uint8
	Inner      InnerDynamic
}

func (obj NestedDynamic) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Serialize `FixedBytes` param:
	err = encoder.Encode(obj.FixedBytes)
	if err != nil {
		return err
	}
	// Serialize `Inner` param:
	err = encoder.Encode(obj.Inner)
	if err != nil {
		return err
	}
	return nil
}

func (obj *NestedDynamic) UnmarshalWithDecoder(decoder *agbinary.Decoder) (err error) {
	// Deserialize `FixedBytes`:
	err = decoder.Decode(&obj.FixedBytes)
	if err != nil {
		return err
	}
	// Deserialize `Inner`:
	err = decoder.Decode(&obj.Inner)
	if err != nil {
		return err
	}
	return nil
}

type InnerStatic struct {
	IntVal int64
	A      solana.PublicKey
}

func (obj InnerStatic) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Serialize `IntVal` param:
	err = encoder.Encode(obj.IntVal)
	if err != nil {
		return err
	}
	// Serialize `A` param:
	err = encoder.Encode(obj.A)
	if err != nil {
		return err
	}
	return nil
}

func (obj *InnerStatic) UnmarshalWithDecoder(decoder *agbinary.Decoder) (err error) {
	// Deserialize `IntVal`:
	err = decoder.Decode(&obj.IntVal)
	if err != nil {
		return err
	}
	// Deserialize `A`:
	err = decoder.Decode(&obj.A)
	if err != nil {
		return err
	}
	return nil
}

type NestedStatic struct {
	FixedBytes [2]uint8
	Inner      InnerStatic
}

func (obj NestedStatic) MarshalWithEncoder(encoder *agbinary.Encoder) (err error) {
	// Serialize `FixedBytes` param:
	err = encoder.Encode(obj.FixedBytes)
	if err != nil {
		return err
	}
	// Serialize `Inner` param:
	err = encoder.Encode(obj.Inner)
	if err != nil {
		return err
	}
	return nil
}

func (obj *NestedStatic) UnmarshalWithDecoder(decoder *agbinary.Decoder) (err error) {
	// Deserialize `FixedBytes`:
	err = decoder.Decode(&obj.FixedBytes)
	if err != nil {
		return err
	}
	// Deserialize `Inner`:
	err = decoder.Decode(&obj.Inner)
	if err != nil {
		return err
	}
	return nil
}

func EncodeRequestToTestItemAsAccount(testStruct interfacetests.TestStruct) TestItemAsAccount {
	return TestItemAsAccount{
		Field:               *testStruct.Field,
		OracleID:            uint8(testStruct.OracleID),
		OracleIDs:           getOracleIDs(testStruct),
		AccountStruct:       getAccountStruct(testStruct),
		Accounts:            getAccounts(testStruct),
		DifferentField:      testStruct.DifferentField,
		BigField:            bigIntToBinInt128(testStruct.BigField),
		NestedDynamicStruct: getNestedDynamic(testStruct),
		NestedStaticStruct:  getNestedStatic(testStruct),
	}
}

func EncodeRequestToTestItemAsEvent(testStruct interfacetests.TestStruct) TestItemAsEvent {
	return TestItemAsEvent{
		Field:               *testStruct.Field,
		OracleID:            uint8(testStruct.OracleID),
		OracleIDs:           getOracleIDs(testStruct),
		AccountStruct:       getAccountStruct(testStruct),
		Accounts:            getAccounts(testStruct),
		DifferentField:      testStruct.DifferentField,
		BigField:            bigIntToBinInt128(testStruct.BigField),
		NestedDynamicStruct: getNestedDynamic(testStruct),
		NestedStaticStruct:  getNestedStatic(testStruct),
	}
}

func EncodeRequestToTestItemAsArgs(testStruct interfacetests.TestStruct) TestItemAsArgs {
	return TestItemAsArgs{
		Field:               *testStruct.Field,
		OracleID:            uint8(testStruct.OracleID),
		OracleIDs:           getOracleIDs(testStruct),
		AccountStruct:       getAccountStruct(testStruct),
		Accounts:            getAccounts(testStruct),
		DifferentField:      testStruct.DifferentField,
		BigField:            bigIntToBinInt128(testStruct.BigField),
		NestedDynamicStruct: getNestedDynamic(testStruct),
		NestedStaticStruct:  getNestedStatic(testStruct),
	}
}

func getOracleIDs(testStruct interfacetests.TestStruct) [32]byte {
	var oracleIDs [32]byte
	for i, v := range testStruct.OracleIDs {
		oracleIDs[i] = byte(v)
	}
	return oracleIDs
}

func getAccountStruct(testStruct interfacetests.TestStruct) AccountStruct {
	k, _ := solana.PublicKeyFromBase58(testStruct.AccountStruct.AccountStr)
	return AccountStruct{
		Account:    solana.PublicKeyFromBytes(testStruct.AccountStruct.Account),
		AccountStr: k,
	}
}

func getAccounts(testStruct interfacetests.TestStruct) []solana.PublicKey {
	accs := make([]solana.PublicKey, len(testStruct.Accounts))
	for i, v := range testStruct.Accounts {
		accs[i] = solana.PublicKeyFromBytes(v)
	}
	return accs
}

func getNestedDynamic(testStruct interfacetests.TestStruct) NestedDynamic {
	return NestedDynamic{
		FixedBytes: testStruct.NestedDynamicStruct.FixedBytes,
		Inner: InnerDynamic{
			IntVal: int64(testStruct.NestedDynamicStruct.Inner.I),
			S:      testStruct.NestedDynamicStruct.Inner.S,
		},
	}
}

func getNestedStatic(testStruct interfacetests.TestStruct) NestedStatic {
	return NestedStatic{
		FixedBytes: testStruct.NestedStaticStruct.FixedBytes,
		Inner: InnerStatic{
			IntVal: int64(testStruct.NestedStaticStruct.Inner.I),
			A:      solana.PublicKeyFromBytes(testStruct.NestedStaticStruct.Inner.A),
		},
	}
}

func bigIntToBinInt128(val *big.Int) agbinary.Int128 {
	return agbinary.Int128{
		Lo: val.Uint64(),
		Hi: new(big.Int).Rsh(val, 64).Uint64(),
	}
}
