package relayinterface

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/tokens"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	ccipocr3common "github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	solanautils "github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

type InnerAccountArgs struct {
	Accounts []*solana.AccountMeta
	Bitmap   uint64
}

type TestAccountArgs struct {
	Inner InnerAccountArgs
}

var testContractIDL = chainwriter.FetchTestContractIDL()

func TestLookup(t *testing.T) {
	t.Run("Resolve fails on a lookup with multiple lookup types", func(t *testing.T) {
		lookupConfig := chainwriter.Lookup{
			AccountConstant: &chainwriter.AccountConstant{
				Name:    "TestAccount",
				Address: "test",
			},
			AccountLookup: &chainwriter.AccountLookup{
				Name:     "TestAccount",
				Location: "test",
			},
		}
		_, err := lookupConfig.Resolve(t.Context(), nil, nil, client.MultiClient{})
		require.Contains(t, err.Error(), "exactly one of AccountConstant, AccountLookup, PDALookups, or AccountsFromLookupTable must be specified, got 2")
	})
}

func TestAccountContant(t *testing.T) {
	t.Run("AccountConstant resolves valid address", func(t *testing.T) {
		expectedAddr := utils.GetRandomPubKey(t)
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  expectedAddr,
				IsSigner:   true,
				IsWritable: true,
			},
		}
		constantConfig := chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
			Name:       "TestAccount",
			Address:    expectedAddr.String(),
			IsSigner:   true,
			IsWritable: true,
		}}
		result, err := constantConfig.AccountConstant.Resolve()
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}

func TestAccountLookups(t *testing.T) {
	t.Run("AccountLookup resolves valid address with just one address", func(t *testing.T) {
		expectedAddr := utils.GetRandomPubKey(t)
		testArgs := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: expectedAddr.Bytes()},
			},
		}
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  expectedAddr,
				IsSigner:   true,
				IsWritable: true,
			},
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Inner.Address",
			IsSigner:   chainwriter.MetaBool{Value: true},
			IsWritable: chainwriter.MetaBool{Value: true},
		}}
		result, err := lookupConfig.AccountLookup.Resolve(testArgs)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("AccountLookup resolves valid address with just multiple addresses", func(t *testing.T) {
		expectedAddr1 := utils.GetRandomPubKey(t)
		expectedAddr2 := utils.GetRandomPubKey(t)

		testArgs := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: expectedAddr1.Bytes()},
				{Address: expectedAddr2.Bytes()},
			},
		}
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  expectedAddr1,
				IsSigner:   true,
				IsWritable: true,
			},
			{
				PublicKey:  expectedAddr2,
				IsSigner:   true,
				IsWritable: true,
			},
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Inner.Address",
			IsSigner:   chainwriter.MetaBool{Value: true},
			IsWritable: chainwriter.MetaBool{Value: true},
		}}
		result, err := lookupConfig.AccountLookup.Resolve(testArgs)
		require.NoError(t, err)
		for i, meta := range result {
			require.Equal(t, expectedMeta[i], meta)
		}
	})

	t.Run("AccountLookup fails when address isn't in args", func(t *testing.T) {
		expectedAddr := utils.GetRandomPubKey(t)

		testArgs := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: expectedAddr.Bytes()},
			},
		}
		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Invalid.Directory",
			IsSigner:   chainwriter.MetaBool{Value: true},
			IsWritable: chainwriter.MetaBool{Value: true},
		}}
		_, err := lookupConfig.AccountLookup.Resolve(testArgs)
		require.ErrorIs(t, err, chainwriter.ErrLookupNotFoundAtLocation)
	})

	t.Run("AccountLookup works with MetaBool bitmap lookups", func(t *testing.T) {
		accounts := [3]*solana.AccountMeta{}

		for i := 0; i < 3; i++ {
			accounts[i] = &solana.AccountMeta{
				PublicKey:  utils.GetRandomPubKey(t),
				IsSigner:   (i)%2 == 0,
				IsWritable: (i)%2 == 0,
			}
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Inner.Accounts.PublicKey",
			IsSigner:   chainwriter.MetaBool{BitmapLocation: "Inner.Bitmap"},
			IsWritable: chainwriter.MetaBool{BitmapLocation: "Inner.Bitmap"},
		}}

		args := TestAccountArgs{
			Inner: InnerAccountArgs{
				Accounts: accounts[:],
				// should be 101... so {true, false, true}
				Bitmap: 5,
			},
		}

		result, err := lookupConfig.AccountLookup.Resolve(args)
		require.NoError(t, err)

		for i, meta := range result {
			require.Equal(t, accounts[i], meta)
		}
	})

	t.Run("AccountLookup fails with MetaBool due to an invalid number of bitmaps", func(t *testing.T) {
		type TestAccountArgsExtended struct {
			Inner   InnerAccountArgs
			Bitmaps []uint64
		}

		accounts := [3]*solana.AccountMeta{}

		for i := 0; i < 3; i++ {
			accounts[i] = &solana.AccountMeta{
				PublicKey:  utils.GetRandomPubKey(t),
				IsWritable: true,
				IsSigner:   true,
			}
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Inner.Accounts.PublicKey",
			IsSigner:   chainwriter.MetaBool{BitmapLocation: "Bitmaps"},
			IsWritable: chainwriter.MetaBool{BitmapLocation: "Bitmaps"},
		}}

		args := TestAccountArgsExtended{
			Inner: InnerAccountArgs{
				Accounts: accounts[:],
			},
			Bitmaps: []uint64{5, 3},
		}

		_, err := lookupConfig.AccountLookup.Resolve(args)
		require.Contains(t, err.Error(), "bitmap value is not a single value")
	})

	t.Run("AccountLookup fails with MetaBool with an Invalid BitmapLocation", func(t *testing.T) {
		accounts := [3]*solana.AccountMeta{}

		for i := 0; i < 3; i++ {
			accounts[i] = &solana.AccountMeta{
				PublicKey:  utils.GetRandomPubKey(t),
				IsWritable: true,
			}
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Inner.Accounts.PublicKey",
			IsSigner:   chainwriter.MetaBool{BitmapLocation: "Invalid.Bitmap"},
			IsWritable: chainwriter.MetaBool{BitmapLocation: "Invalid.Bitmap"},
		}}

		args := TestAccountArgs{
			Inner: InnerAccountArgs{
				Accounts: accounts[:],
			},
		}

		_, err := lookupConfig.AccountLookup.Resolve(args)
		require.Contains(t, err.Error(), "error reading bitmap from location")
	})

	t.Run("AccountLookup fails when MetaBool Bitmap is an invalid type", func(t *testing.T) {
		accounts := [3]*solana.AccountMeta{}

		for i := 0; i < 3; i++ {
			accounts[i] = &solana.AccountMeta{
				PublicKey:  utils.GetRandomPubKey(t),
				IsWritable: true,
			}
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Inner.Accounts.PublicKey",
			IsSigner:   chainwriter.MetaBool{BitmapLocation: "Inner"},
			IsWritable: chainwriter.MetaBool{BitmapLocation: "Inner"},
		}}

		args := TestAccountArgs{
			Inner: InnerAccountArgs{
				Accounts: accounts[:],
			},
		}

		_, err := lookupConfig.AccountLookup.Resolve(args)
		require.Contains(t, err.Error(), "invalid value format at path")
	})

	t.Run("AccountLookup fails if lookup returns more than 64 values and uses bitmap", func(t *testing.T) {
		accounts := [65]*solana.AccountMeta{}

		for i := 0; i < 65; i++ {
			accounts[i] = &solana.AccountMeta{
				PublicKey:  utils.GetRandomPubKey(t),
				IsWritable: true,
			}
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Inner.Accounts.PublicKey",
			IsSigner:   chainwriter.MetaBool{Value: true},
			IsWritable: chainwriter.MetaBool{Value: true},
		}}
		args := TestAccountArgs{
			Inner: InnerAccountArgs{
				Accounts: accounts[:],
			},
		}
		_, err := lookupConfig.AccountLookup.Resolve(args)
		require.Error(t, err)
	})

	t.Run("AccountLookup fails if provided bitmap is not 8 bytes", func(t *testing.T) {
		accounts := [3]*solana.AccountMeta{}

		for i := 0; i < 3; i++ {
			accounts[i] = &solana.AccountMeta{
				PublicKey:  utils.GetRandomPubKey(t),
				IsSigner:   (i)%2 == 0,
				IsWritable: (i)%2 == 0,
			}
		}

		lookupConfig := chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Accounts.PublicKey",
			IsSigner:   chainwriter.MetaBool{BitmapLocation: "Bitmap"},
			IsWritable: chainwriter.MetaBool{BitmapLocation: "Bitmap"},
		}}

		args := struct {
			Accounts []*solana.AccountMeta
			Bitmap   []byte
		}{
			Accounts: accounts[:],
			Bitmap:   make([]byte, 3), // invalid bitmap length
		}

		_, err := lookupConfig.AccountLookup.Resolve(args)
		require.Error(t, err)
	})
}

func TestPDALookups(t *testing.T) {
	programID := utils.GetRandomPubKey(t)
	ctx := t.Context()

	t.Run("PDALookup resolves valid PDA with constant address seeds", func(t *testing.T) {
		seed := utils.GetRandomPubKey(t)

		pda, _, err := solana.FindProgramAddress([][]byte{seed.Bytes()}, programID)
		require.NoError(t, err)

		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: true,
			},
		}

		pdaLookup := chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()}},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "seed", Address: seed.String()}}},
			},
			IsSigner:   false,
			IsWritable: true,
		}}

		result, err := pdaLookup.Resolve(ctx, nil, nil, client.MultiClient{})
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
	t.Run("PDALookup resolves valid PDA with non-address lookup seeds", func(t *testing.T) {
		seed1 := []byte("test_seed")
		seed2 := uint64(4)
		bufSeed2 := make([]byte, 8)
		binary.LittleEndian.PutUint64(bufSeed2, seed2)
		seed3 := ccipocr3common.ChainSelector(4)
		bufSeed3 := make([]byte, 8)
		binary.LittleEndian.PutUint64(bufSeed3, uint64(seed3))
		seed4 := ccipocr3common.Bytes32(utils.GetRandomPubKey(t).Bytes())

		pda, _, err := solana.FindProgramAddress([][]byte{seed1, bufSeed2, bufSeed3, seed4[:]}, programID)
		require.NoError(t, err)

		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: true,
			},
		}

		pdaLookup := chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()}},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed1", Location: "test_seed"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed2", Location: "another_seed"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed3", Location: "ccip_chain_selector"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed4", Location: "ccip_bytes"}}},
			},
			IsSigner:   false,
			IsWritable: true,
		}}

		args := map[string]interface{}{
			"test_seed":           seed1,
			"another_seed":        seed2,
			"ccip_chain_selector": seed3,
			"ccip_bytes":          seed4,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, client.MultiClient{})
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookup fails with missing seeds", func(t *testing.T) {
		pdaLookup := chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()}},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed1", Location: "MissingSeed"}}},
			},
			IsSigner:   false,
			IsWritable: true,
		}}

		args := map[string]interface{}{
			"test_seed": []byte("data"),
		}

		_, err := pdaLookup.Resolve(ctx, args, nil, client.MultiClient{})
		require.ErrorIs(t, err, chainwriter.ErrGettingSeedAtLocation)
	})

	t.Run("PDALookup resolves valid PDA with address lookup seeds", func(t *testing.T) {
		seed1 := utils.GetRandomPubKey(t)
		seed2 := utils.GetRandomPubKey(t)
		addr3 := utils.GetRandomPubKey(t)
		seed3 := ccipocr3common.UnknownEncodedAddress(addr3.String())

		pda, _, err := solana.FindProgramAddress([][]byte{seed1.Bytes(), seed2.Bytes(), addr3.Bytes()}, programID)
		require.NoError(t, err)

		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: true,
			},
		}

		pdaLookup := chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()}},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed1", Location: "test_seed"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed2", Location: "another_seed"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed3", Location: "unknown_encoded_address"}}},
			},
			IsSigner:   false,
			IsWritable: true,
		}}

		args := map[string]interface{}{
			"test_seed":               seed1,
			"another_seed":            seed2,
			"unknown_encoded_address": seed3,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, client.MultiClient{})
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookups resolves list of PDAs when a seed is an array", func(t *testing.T) {
		singleSeed := []byte("test_seed")
		arraySeed := []solana.PublicKey{utils.GetRandomPubKey(t), utils.GetRandomPubKey(t)}

		expectedMeta := []*solana.AccountMeta{}

		for _, seed := range arraySeed {
			pda, _, err := solana.FindProgramAddress([][]byte{singleSeed, seed.Bytes()}, programID)
			require.NoError(t, err)
			meta := &solana.AccountMeta{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: false,
			}
			expectedMeta = append(expectedMeta, meta)
		}

		pdaLookup := chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()}},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed1", Location: "single_seed"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed2", Location: "array_seed"}}},
			},
			IsSigner:   false,
			IsWritable: false,
		}}

		args := map[string]interface{}{
			"single_seed": singleSeed,
			"array_seed":  arraySeed,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, client.MultiClient{})
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookups resolves list of PDAs when multiple seeds are arrays", func(t *testing.T) {
		arraySeed1 := [][]byte{[]byte("test_seed1"), []byte("test_seed2")}
		arraySeed2 := []solana.PublicKey{utils.GetRandomPubKey(t), utils.GetRandomPubKey(t)}

		expectedMeta := []*solana.AccountMeta{}

		for _, seed1 := range arraySeed1 {
			for _, seed2 := range arraySeed2 {
				pda, _, err := solana.FindProgramAddress([][]byte{seed1, seed2.Bytes()}, programID)
				require.NoError(t, err)
				meta := &solana.AccountMeta{
					PublicKey:  pda,
					IsSigner:   false,
					IsWritable: false,
				}
				expectedMeta = append(expectedMeta, meta)
			}
		}

		pdaLookup := chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()}},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed1", Location: "seed1"}}},
				{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed2", Location: "seed2"}}},
			},
			IsSigner:   false,
			IsWritable: false,
		}}

		args := map[string]interface{}{
			"seed1": arraySeed1,
			"seed2": arraySeed2,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, client.MultiClient{})
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}

func TestLookupTables(t *testing.T) {
	ctx := t.Context()

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, sender.PublicKey().String(), []string{"contract-reader-interface"})
	rpcClient := rpc.New(url)

	utils.FundAccounts(t, []solana.PrivateKey{sender}, rpcClient)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, nil)
	require.NoError(t, err)

	multiClient := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return solanaClient, nil
	})

	loader := solanautils.NewStaticLoader[client.ReaderWriter](solanaClient)
	mkey := keyMocks.NewSimpleKeystore(t)
	lggr := logger.Test(t)

	txm, err := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)
	require.NoError(t, err)

	cw, err := chainwriter.NewSolanaChainWriterService(logger.Test(t), multiClient, txm, nil, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	t.Run("StaticLookup table resolves properly", func(t *testing.T) {
		pubKeys := CreateTestPubKeys(t, 8)
		table := utils.CreateTestLookupTable(t, rpcClient, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: nil,
			StaticLookupTables:  []solana.PublicKey{table},
		}
		_, staticTableMap, resolveErr := cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.NoError(t, resolveErr)
		require.Equal(t, pubKeys, staticTableMap[table])
	})
	t.Run("Derived lookup table resolves properly with constant address", func(t *testing.T) {
		pubKeys := CreateTestPubKeys(t, 8)
		table := utils.CreateTestLookupTable(t, rpcClient, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
						Name:       "TestLookupTable",
						Address:    table.String(),
						IsSigner:   true,
						IsWritable: true,
					}},
				},
			},
			StaticLookupTables: nil,
		}
		derivedTableMap, _, resolveErr := cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.NoError(t, resolveErr)

		addresses, ok := derivedTableMap["DerivedTable"][table.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})

	t.Run("Derived lookup table fails with invalid address", func(t *testing.T) {
		invalidTable := utils.GetRandomPubKey(t)

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{
						AccountConstant: &chainwriter.AccountConstant{
							Name:       "InvalidTable",
							Address:    invalidTable.String(),
							IsSigner:   true,
							IsWritable: true,
						},
					},
				},
			},
			StaticLookupTables: nil,
		}

		_, _, err = cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error fetching account info for table") // Example error message
	})

	t.Run("Derived lookup table fails with invalid table name", func(t *testing.T) {
		derivedTableMap := map[string]map[string][]*solana.AccountMeta{
			"DerivedTable": {},
		}
		accountsFromLookupTable := chainwriter.Lookup{
			AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "InvalidTable",
				IncludeIndexes:  []int{},
			},
		}

		_, err = accountsFromLookupTable.Resolve(ctx, nil, derivedTableMap, multiClient)
		require.ErrorIs(t, err, chainwriter.ErrLookupTableNotFound)
	})

	t.Run("Static lookup table fails with invalid address", func(t *testing.T) {
		invalidTable := utils.GetRandomPubKey(t)

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: nil,
			StaticLookupTables:  []solana.PublicKey{invalidTable},
		}

		_, _, err = cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error fetching account info for table") // Example error message
	})

	t.Run("Derived lookup table resolves properly with account lookup address", func(t *testing.T) {
		pubKeys := CreateTestPubKeys(t, 8)
		table := utils.CreateTestLookupTable(t, rpcClient, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
						Name:     "TestLookupTable",
						Location: "Inner.Address",
						IsSigner: chainwriter.MetaBool{Value: true},
					}},
				},
			},
			StaticLookupTables: nil,
		}

		testArgs := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: table.Bytes()},
			},
		}

		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, testArgs, lookupConfig)
		require.NoError(t, err)

		accountsFromLookupTable := chainwriter.Lookup{
			AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{},
			},
		}

		addresses, err := accountsFromLookupTable.Resolve(ctx, nil, derivedTableMap, multiClient)
		require.NoError(t, err)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})

	t.Run("Derived lookup table resolves properly with PDALookup address", func(t *testing.T) {
		// Deployed contract_reader_interface contract
		programID := solana.MustPublicKeyFromBase58("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE")

		lookupKeys := CreateTestPubKeys(t, 5)
		lookupTable := utils.CreateTestLookupTable(t, rpcClient, sender, lookupKeys)

		InitializeDataAccount(ctx, t, rpcClient, programID, sender, lookupTable)

		args := map[string]interface{}{
			"seed1": []byte("lookup"),
		}

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{
						PDALookups: &chainwriter.PDALookups{
							Name:      "DataAccountPDA",
							PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()}},
							Seeds: []chainwriter.Seed{
								{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "seed1", Location: "seed1"}}},
							},
							IsSigner:   false,
							IsWritable: false,
							InternalField: chainwriter.InternalField{
								TypeName: "LookupTableDataAccount",
								Location: "LookupTable",
								IDL:      testContractIDL,
							},
						},
					},
				},
			},
			StaticLookupTables: nil,
		}

		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupConfig)
		require.NoError(t, err)

		accountsFromLookupTable := chainwriter.Lookup{
			AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{},
			},
		}

		addresses, err := accountsFromLookupTable.Resolve(ctx, args, derivedTableMap, multiClient)
		require.NoError(t, err)
		for i, address := range addresses {
			require.Equal(t, lookupKeys[i], address.PublicKey)
		}
	})

	t.Run("Resolving optional derived lookup table does not return error", func(t *testing.T) {
		// Deployed contract_reader_interface contract
		programID := solana.MustPublicKeyFromBase58("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE")

		args := map[string]interface{}{
			"seed1": []byte("lookup"),
		}

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{
						PDALookups: &chainwriter.PDALookups{
							Name:      "DataAccountPDA",
							PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()}},
							Seeds: []chainwriter.Seed{
								{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "missing_seed", Location: "missing_seed"}}},
							},
							IsSigner:   false,
							IsWritable: false,
							InternalField: chainwriter.InternalField{
								TypeName: "LookupTableDataAccount",
								Location: "LookupTable",
								IDL:      testContractIDL,
							},
						},
					},
					Optional: true,
				},
			},
		}

		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupConfig)
		require.NoError(t, err)

		pdaWithAccountLookupSeed := chainwriter.Lookup{
			PDALookups: &chainwriter.PDALookups{
				PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Address: utils.GetRandomPubKey(t).String()}},
				Seeds: []chainwriter.Seed{
					{
						Dynamic: chainwriter.Lookup{
							AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
								LookupTableName: "DerivedTable",
								IncludeIndexes:  []int{},
							},
						},
					},
				},
			},
			Optional: true,
		}

		accounts, err := chainwriter.GetAddresses(ctx, nil, []chainwriter.Lookup{pdaWithAccountLookupSeed}, derivedTableMap, multiClient)
		require.NoError(t, err)
		require.Empty(t, accounts)
	})
}

func TestCreateATAs(t *testing.T) {
	ctx := t.Context()
	lggr := logger.Test(t)

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	feePayer := sender.PublicKey()

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, feePayer.String(), nil)
	cfg := config.NewDefault()
	solanaClient, rpcClient, err := client.NewTestClient(url, cfg, 5*time.Second, nil)
	require.NoError(t, err)

	utils.FundAccounts(t, []solana.PrivateKey{sender}, rpcClient)

	multiClient := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return solanaClient, nil
	})

	// initialize two mints
	mint1PK, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	mint1 := mint1PK.PublicKey()
	mint2PK, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	mint2 := mint2PK.PublicKey()

	tokenProgram := solana.Token2022ProgramID
	mint1Ixs, err := tokens.CreateToken(ctx, tokenProgram, mint1, feePayer, 9, rpcClient, rpc.CommitmentConfirmed)
	require.NoError(t, err)
	mint2Ixs, err := tokens.CreateToken(ctx, tokenProgram, mint2, feePayer, 9, rpcClient, rpc.CommitmentConfirmed)
	require.NoError(t, err)

	signers := make(map[solana.PublicKey]solana.PrivateKey)
	signers[sender.PublicKey()] = sender
	signers[mint1] = mint1PK
	signers[mint2] = mint2PK

	res, err := multiClient.LatestBlockhash(ctx)
	require.NoError(t, err)

	mint1TX, err := solana.NewTransaction(mint1Ixs, res.Value.Blockhash, solana.TransactionPayer(feePayer))
	require.NoError(t, err)
	_, err = mint1TX.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		pk := signers[key]
		return &pk
	})
	require.NoError(t, err)

	mint2TX, err := solana.NewTransaction(mint2Ixs, res.Value.Blockhash, solana.TransactionPayer(feePayer))
	require.NoError(t, err)
	_, err = mint2TX.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		pk := signers[key]
		return &pk
	})
	require.NoError(t, err)

	mint1Sig, err := multiClient.SendTx(ctx, mint1TX)
	require.NoError(t, err)
	mint2Sig, err := multiClient.SendTx(ctx, mint2TX)
	require.NoError(t, err)

	sigs := []solana.Signature{mint1Sig, mint2Sig}

	require.Eventually(t, func() bool {
		statuses, err := multiClient.SignatureStatuses(ctx, sigs)
		require.NoError(t, err)
		if res == nil || len(statuses) < len(sigs) {
			return false
		}
		for _, status := range statuses {
			if status == nil {
				return false
			}
			if status.Err != nil {
				require.Fail(t, fmt.Sprintf("%v", status.Err))
			}
			// Wait till finality otherwise ATA creation may error with invalid mint
			if status.ConfirmationStatus != rpc.ConfirmationStatusFinalized {
				return false
			}
		}
		return true
	}, 1*time.Minute, time.Second, "failed to confirm create mint transaction")

	t.Run("returns no instructions when no ATA location is found", func(t *testing.T) {
		user := utils.GetRandomPubKey(t)
		lookups := []chainwriter.ATALookup{
			{
				Location: "Invalid.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: user.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Invalid.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: mint1.Bytes()},
			},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.NoError(t, err)
		require.Empty(t, ataInstructions)
	})

	t.Run("fails with multiple wallet addresses", func(t *testing.T) {
		lookups := []chainwriter.ATALookup{
			{
				Location: "",
				WalletAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Addresses",
				}},
				MintAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: mint1.String(),
				}},
			},
		}

		args := map[string][]solana.PublicKey{
			"Addresses": {utils.GetRandomPubKey(t), utils.GetRandomPubKey(t)},
		}

		_, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.Contains(t, err.Error(), "expected exactly one wallet address, got 2")
	})

	t.Run("fails when mint is not a token address", func(t *testing.T) {
		user := utils.GetRandomPubKey(t)
		ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint1, user)
		require.NoError(t, err)
		require.False(t, checkIfATAExists(t, rpcClient, ataAddress))
		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: user.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: utils.GetRandomPubKey(t).Bytes()},
			},
		}

		_, err = chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.ErrorContains(t, err, "failed to fetch account info for token mint")
	})

	t.Run("successfully creates ATAs only when necessary", func(t *testing.T) {
		user := utils.GetRandomPubKey(t)
		ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint1, user)
		require.NoError(t, err)
		require.False(t, checkIfATAExists(t, rpcClient, ataAddress))
		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: user.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: mint1.Bytes()},
			},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.NoError(t, err)

		utils.SendAndConfirm(ctx, t, rpcClient, ataInstructions, sender, rpc.CommitmentFinalized)
		require.True(t, checkIfATAExists(t, rpcClient, ataAddress))

		// now, if we try to create the same ATA again, it should return no instructions
		ataInstructions, err = chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.NoError(t, err)
		require.Empty(t, ataInstructions)
	})

	t.Run("successfully creates multiple ATAs when necessary", func(t *testing.T) {
		mints := []solana.PublicKey{mint1, mint2}
		user := utils.GetRandomPubKey(t)

		var ataAddresses []solana.PublicKey
		for _, mint := range mints {
			ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, user)
			require.NoError(t, err)
			require.False(t, checkIfATAExists(t, rpcClient, ataAddress), "ATA should not exist yet")
			ataAddresses = append(ataAddresses, ataAddress)
		}

		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: user.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{},
		}
		for _, mint := range mints {
			args.Inner = append(args.Inner, chainwriter.InnerArgs{
				Address:       mint.Bytes(),
				SecondAddress: tokenProgram.Bytes(),
			})
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.NoError(t, err)
		require.Len(t, ataInstructions, len(mints))

		utils.SendAndConfirm(ctx, t, rpcClient, ataInstructions, sender, rpc.CommitmentFinalized)

		for _, ataAddress := range ataAddresses {
			require.True(t, checkIfATAExists(t, rpcClient, ataAddress), "ATA should have been created")
		}

		ataInstructions, err = chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.NoError(t, err)
		require.Empty(t, ataInstructions, "No new instructions should be returned if ATAs already exist")
	})

	t.Run("optional ATA creation does not return error if lookups fail", func(t *testing.T) {
		user := utils.GetRandomPubKey(t)
		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: user.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.BadLocation",
				}},
				Optional: true,
			},
		}
		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{{Address: mint1.Bytes()}},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, lggr)
		require.NoError(t, err)
		require.Len(t, ataInstructions, 0)
	})
}

func checkIfATAExists(t *testing.T, rpcClient *rpc.Client, ataAddress solana.PublicKey) bool {
	_, err := rpcClient.GetAccountInfo(t.Context(), ataAddress)
	return err == nil
}

func InitializeDataAccount(
	ctx context.Context,
	t *testing.T,
	client *rpc.Client,
	programID solana.PublicKey,
	admin solana.PrivateKey,
	lookupTable solana.PublicKey,
) {
	t.Helper()
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("lookup")}, programID)
	require.NoError(t, err)

	discriminator := solcommoncodec.NewMethodDiscriminatorHashPrefix("initializelookuptable")

	instructionData := append(discriminator[:], lookupTable.Bytes()...)

	instruction := solana.NewInstruction(
		programID,
		solana.AccountMetaSlice{
			solana.Meta(admin.PublicKey()).SIGNER().WRITE(),
			solana.Meta(pda).WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		instructionData,
	)

	// Send and confirm the transaction
	utils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, rpc.CommitmentFinalized)
}

func GetRandomPubKey(t *testing.T) solana.PublicKey {
	t.Helper()
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey()
}

func CreateTestPubKeys(t *testing.T, num int) solana.PublicKeySlice {
	t.Helper()

	addresses := make([]solana.PublicKey, num)
	for i := 0; i < num; i++ {
		addresses[i] = utils.GetRandomPubKey(t)
	}
	return addresses
}
