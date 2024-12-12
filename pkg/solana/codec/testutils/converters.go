package testutils

import (
	"math/big"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/libocr/commontypes"

	"github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils/test_item_type"
)

func EncodeRequestToTestItem(request *interfacetests.EncodeRequest) test_item_type.TestItem {
	byt := [32]byte{}
	for i, v := range request.TestStructs[0].OracleIDs {
		byt[i] = byte(v)
	}

	k, _ := solana.PublicKeyFromBase58(request.TestStructs[0].AccountStruct.AccountStr)

	accs := make([]solana.PublicKey, len(request.TestStructs[0].Accounts))
	for i, v := range request.TestStructs[0].Accounts {
		accs[i] = solana.PublicKeyFromBytes(v)
	}

	testItem := test_item_type.TestItem{
		Field:     *request.TestStructs[0].Field,
		OracleId:  uint8(request.TestStructs[0].OracleID),
		OracleIds: byt,
		AccountStruct: test_item_type.AccountStruct{
			Account:    solana.PublicKeyFromBytes(request.TestStructs[0].AccountStruct.Account),
			AccountStr: k,
		},
		Accounts:       accs,
		DifferentField: request.TestStructs[0].DifferentField,
		BigField:       bigIntToBinInt128(request.TestStructs[0].BigField),
		NestedDynamicStruct: test_item_type.NestedDynamic{
			FixedBytes: request.TestStructs[0].NestedDynamicStruct.FixedBytes,
			Inner: test_item_type.InnerDynamic{
				IntVal: int64(request.TestStructs[0].NestedDynamicStruct.Inner.I),
				S:      request.TestStructs[0].NestedDynamicStruct.Inner.S,
			},
		},
		NestedStaticStruct: test_item_type.NestedStatic{
			FixedBytes: request.TestStructs[0].NestedStaticStruct.FixedBytes,
			Inner: test_item_type.InnerStatic{
				IntVal: int64(request.TestStructs[0].NestedStaticStruct.Inner.I),
				A:      solana.PublicKeyFromBytes(request.TestStructs[0].NestedStaticStruct.Inner.A),
			},
		},
	}
	return testItem
}

func bigIntToBinInt128(val *big.Int) bin.Int128 {
	return bin.Int128{
		Lo: val.Uint64(),
		Hi: new(big.Int).Rsh(val, 64).Uint64(),
	}
}

func argsFromTestStruct(ts interfacetests.TestStruct) []any {
	return []any{
		ts.Field,
		ts.DifferentField,
		uint8(ts.OracleID),
		getOracleIDs(ts),
		accountStructToInternalType(ts.AccountStruct),
		getAccounts(ts),
		ts.BigField,
		midDynamicToInternalType(ts.NestedDynamicStruct),
		midStaticToInternalType(ts.NestedStaticStruct),
	}
}

func getOracleIDs(first interfacetests.TestStruct) [32]byte {
	oracleIDs := [32]byte{}
	for i, oracleID := range first.OracleIDs {
		oracleIDs[i] = byte(oracleID)
	}
	return oracleIDs
}

func oracleIDsToBytes(oracleIDs [32]commontypes.OracleID) [32]byte {
	convertedIDs := [32]byte{}
	for i, id := range oracleIDs {
		convertedIDs[i] = byte(id)
	}
	return convertedIDs
}

func ToInternalType(testItem interfacetests.TestStruct) test_item_type.TestItem {
	return test_item_type.TestItem{
		Field:               *testItem.Field,
		DifferentField:      testItem.DifferentField,
		OracleId:            byte(testItem.OracleID),
		OracleIds:           oracleIDsToBytes(testItem.OracleIDs),
		AccountStruct:       accountStructToInternalType(testItem.AccountStruct),
		Accounts:            convertAccounts(testItem.Accounts),
		BigField:            bigIntToBinInt128(testItem.BigField),
		NestedDynamicStruct: midDynamicToInternalType(testItem.NestedDynamicStruct),
		NestedStaticStruct:  midStaticToInternalType(testItem.NestedStaticStruct),
	}
}

func accountStructToInternalType(a interfacetests.AccountStruct) test_item_type.AccountStruct {
	return test_item_type.AccountStruct{
		Account:    solana.PublicKeyFromBytes(a.Account),
		AccountStr: solana.MustPublicKeyFromBase58(a.AccountStr),
	}
}

func convertAccounts(accounts [][]byte) []solana.PublicKey {
	convertedAccounts := make([]solana.PublicKey, len(accounts))
	for i, a := range accounts {
		convertedAccounts[i] = solana.PublicKeyFromBytes(a)
	}
	return convertedAccounts
}

func midDynamicToInternalType(m interfacetests.MidLevelDynamicTestStruct) test_item_type.NestedDynamic {
	return test_item_type.NestedDynamic{
		FixedBytes: m.FixedBytes,
		Inner: test_item_type.InnerDynamic{
			IntVal: int64(m.Inner.I),
			S:      m.Inner.S,
		},
	}
}

func midStaticToInternalType(m interfacetests.MidLevelStaticTestStruct) test_item_type.NestedStatic {
	return test_item_type.NestedStatic{
		FixedBytes: m.FixedBytes,
		Inner: test_item_type.InnerStatic{
			IntVal: int64(m.Inner.I),
			A:      solana.PublicKeyFromBytes(m.Inner.A),
		},
	}
}

func getAccounts(first interfacetests.TestStruct) []solana.PublicKey {
	accountBytes := make([]solana.PublicKey, len(first.Accounts))
	for i, account := range first.Accounts {
		accountBytes[i] = solana.PublicKeyFromBytes(account)
	}
	return accountBytes
}
