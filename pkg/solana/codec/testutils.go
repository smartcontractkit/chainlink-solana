package codec

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/test-go/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
)

var (
	TestStructWithNestedStruct = "StructWithNestedStruct"
	DefaultStringRef           = "test string"
	DefaultTestStruct          = StructWithNestedStruct{
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
		PublicKey:   ag_solana.NewWallet().PublicKey(),
		EnumVal:     0,
	}
)

// NewTestIDLAndCodec creates a complete IDL that covers all types and is exported here to allow parent packages to
// use for testing.
func NewTestIDLAndCodec(t *testing.T) (string, IDL, encodings.CodecFromTypeCodec) {
	t.Helper()

	var idl IDL
	if err := json.Unmarshal([]byte(jsonIDLWithAllTypes), &idl); err != nil {
		t.Logf("failed to unmarshal test IDL: %s", err.Error())
		t.FailNow()
	}

	entry, err := NewIDLCodec(idl)
	if err != nil {
		t.Logf("failed to create new codec from test IDL: %s", err.Error())
		t.FailNow()
	}

	require.NotNil(t, entry)

	return jsonIDLWithAllTypes, idl, entry
}

type StructWithNestedStruct struct {
	Value            uint8
	InnerStruct      ObjectRef1
	BasicNestedArray [][]uint32
	Option           *string
	DefinedArray     []ObjectRef2
	BasicVector      []string
	TimeVal          int64
	DurationVal      time.Duration
	PublicKey        ag_solana.PublicKey
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

const jsonIDLWithAllTypes = `{
		"version": "0.1.0",
		"name": "some_test_idl",
		"accounts": [
			{
				"name": "StructWithNestedStruct",
				"type": {
					"kind": "struct",
					"fields": [
						{
							"name": "value",
							"type": "u8"
						},
						{
							"name": "innerStruct",
							"type": {
								"defined": "ObjectRef1"
							}
						},
						{
							"name": "basicNestedArray",
							"type": {
								"array": [
									{
										"array": [
											"u32",
											3
										]
									},
									3
								]
							}
						},
						{
							"name": "option",
							"type": {
								"option": "string"
							}
						},
						{
							"name": "definedArray",
							"type": {
								"array": [
									{
										"defined": "ObjectRef2"
									},
									2
								]
							}
						},
						{
							"name": "basicVector",
							"type": {
								"vec": "string"
							}
						},
						{
							"name": "timeVal",
							"type": "unixTimestamp"
						},
						{
							"name": "durationVal",
							"type": "duration"
						},
						{
							"name": "publicKey",
							"type": "publicKey"
						},
						{
							"name": "enumVal",
							"type": {
								"defined": "SimpleEnum"
							}
						}
					]
				}
			}
		],
		"types": [
			{
				"name": "ObjectRef1",
				"type": {
					"kind": "struct",
					"fields": [
						{
							"name": "prop1",
							"type": "i8"
						},
						{
							"name": "prop2",
							"type": "string"
						},
						{
							"name": "prop3",
							"type": "u128"
						},
						{
							"name": "prop4",
							"type": "u16"
						},
						{
							"name": "prop5",
							"type": "u64"
						},
						{
							"name": "prop6",
							"type": "bool"
						}
					]
				}
			},
			{
				"name": "ObjectRef2",
				"type": {
					"kind": "struct",
					"fields": [
						{
							"name": "prop1",
							"type": "u32"
						},
						{
							"name": "prop2",
							"type": "i128"
						},
						{
							"name": "prop3",
							"type": "i16"
						},
						{
							"name": "prop4",
							"type": "i32"
						},
						{
							"name": "prop5",
							"type": "i64"
						},
						{
							"name": "prop6",
							"type": "bytes"
						}
					]
				}
			},
			{
				"name": "SimpleEnum",
				"type": {
					"kind": "enum",
					"variants": [
						{
							"name": "A"
						},
						{
							"name": "B"
						}
					]
				}
			}
		]
	}`
