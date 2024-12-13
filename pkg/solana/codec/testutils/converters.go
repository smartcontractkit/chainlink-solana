package testutils

import (
	"math/big"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests"
)

func EncodeRequestToTestItemAsAccount(testStruct interfacetests.TestStruct) TestItemAsAccount {
	return TestItemAsAccount{
		Field:               *testStruct.Field,
		OracleId:            uint8(testStruct.OracleID),
		OracleIds:           getOracleIds(testStruct),
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
		OracleId:            uint8(testStruct.OracleID),
		OracleIds:           getOracleIds(testStruct),
		AccountStruct:       getAccountStruct(testStruct),
		Accounts:            getAccounts(testStruct),
		DifferentField:      testStruct.DifferentField,
		BigField:            bigIntToBinInt128(testStruct.BigField),
		NestedDynamicStruct: getNestedDynamic(testStruct),
		NestedStaticStruct:  getNestedStatic(testStruct),
	}
}

func getOracleIds(testStruct interfacetests.TestStruct) [32]byte {
	var oracleIds [32]byte
	for i, v := range testStruct.OracleIDs {
		oracleIds[i] = byte(v)
	}
	return oracleIds
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

func bigIntToBinInt128(val *big.Int) bin.Int128 {
	return bin.Int128{
		Lo: val.Uint64(),
		Hi: new(big.Int).Rsh(val, 64).Uint64(),
	}
}
