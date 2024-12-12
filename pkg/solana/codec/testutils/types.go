package testutils

import (
	_ "embed"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
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
	TestItemType            = "TestItem"
	TestItemSliceType       = "TestItemSliceType"
	TestItemArray1Type      = "TestItemArray1Type"
	TestItemArray2Type      = "TestItemArray2Type"
	TestItemWithConfigExtra = "TestItemWithConfigExtra"
	NilType                 = "NilType"
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

const SizeItemType = "item for size"

//go:embed test_item_type/itemIDL.json
var itemTypeJSONIDL string

//go:embed test_item_slice_type/itemSliceTypeIDL.json
var itemSliceTypeJSONIDL string

//go:embed itemArray1TypeIDL.json
var itemArray1TypeJSONIDL string

//go:embed itemArray2TypeIDL.json
var itemArray2TypeJSONIDL string

//go:embed sizeItemTypeIDL.json
var sizeItemTypeJSONIDL string

//go:embed itemWithConfigExtraIDL.json
var itemWithConfigExtraJSONIDL string

//go:embed nilTypeIDL.json
var nilTypeJSONIDL string

var CodecDefs = map[string]string{
	//TestItemType:      itemTypeJSONIDL,
	TestItemSliceType: itemSliceTypeJSONIDL,
	//TestItemArray1Type:      itemArray1TypeJSONIDL,
	//TestItemArray2Type:      itemArray2TypeJSONIDL,
	//sizeItemType:            sizeItemTypeJSONIDL,
	//TestItemWithConfigExtra: itemWithConfigExtraJSONIDL,
	//NilType:                 nilTypeJSONIDL,
}
