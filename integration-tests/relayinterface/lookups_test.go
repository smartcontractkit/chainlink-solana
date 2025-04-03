package relayinterface

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
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
		expectedAddr := solanautils.GetRandomPubKey(t)
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
		expectedAddr := solanautils.GetRandomPubKey(t)
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
		expectedAddr1 := solanautils.GetRandomPubKey(t)
		expectedAddr2 := solanautils.GetRandomPubKey(t)

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
		expectedAddr := solanautils.GetRandomPubKey(t)

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
				PublicKey:  solanautils.GetRandomPubKey(t),
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
				PublicKey:  solanautils.GetRandomPubKey(t),
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
				PublicKey:  solanautils.GetRandomPubKey(t),
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
				PublicKey:  solanautils.GetRandomPubKey(t),
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
}

func TestPDALookups(t *testing.T) {
	programID := solanautils.GetRandomPubKey(t)
	ctx := t.Context()

	t.Run("PDALookup resolves valid PDA with constant address seeds", func(t *testing.T) {
		seed := solanautils.GetRandomPubKey(t)

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
		seed3 := ccipocr3.ChainSelector(4)
		bufSeed3 := make([]byte, 8)
		binary.LittleEndian.PutUint64(bufSeed3, uint64(seed3))
		seed4 := ccipocr3.Bytes32(solanautils.GetRandomPubKey(t).Bytes())

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
		seed1 := solanautils.GetRandomPubKey(t)
		seed2 := solanautils.GetRandomPubKey(t)
		addr3 := solanautils.GetRandomPubKey(t)
		seed3 := ccipocr3.UnknownEncodedAddress(addr3.String())

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
		arraySeed := []solana.PublicKey{solanautils.GetRandomPubKey(t), solanautils.GetRandomPubKey(t)}

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
		arraySeed2 := []solana.PublicKey{solanautils.GetRandomPubKey(t), solanautils.GetRandomPubKey(t)}

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

	txm := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)

	cw, err := chainwriter.NewSolanaChainWriterService(logger.Test(t), multiClient, txm, nil, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	t.Run("StaticLookup table resolves properly", func(t *testing.T) {
		pubKeys := chainwriter.CreateTestPubKeys(t, 8)
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
		pubKeys := chainwriter.CreateTestPubKeys(t, 8)
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
		invalidTable := solanautils.GetRandomPubKey(t)

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
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
		invalidTable := solanautils.GetRandomPubKey(t)

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: nil,
			StaticLookupTables:  []solana.PublicKey{invalidTable},
		}

		_, _, err = cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error fetching account info for table") // Example error message
	})

	t.Run("Derived lookup table resolves properly with account lookup address", func(t *testing.T) {
		pubKeys := chainwriter.CreateTestPubKeys(t, 8)
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

		lookupKeys := chainwriter.CreateTestPubKeys(t, 5)
		lookupTable := utils.CreateTestLookupTable(t, rpcClient, sender, lookupKeys)

		chainwriter.InitializeDataAccount(ctx, t, rpcClient, programID, sender, lookupTable)

		args := map[string]interface{}{
			"seed1": []byte("lookup"),
		}

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
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
						}},
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
					Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
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
						}},
					},
					Optional: true,
				},
			},
		}

		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupConfig)
		require.NoError(t, err)

		pdaWithAccountLookupSeed := chainwriter.Lookup{
			PDALookups: &chainwriter.PDALookups{
				PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Address: solanautils.GetRandomPubKey(t).String()}},
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

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	feePayer := sender.PublicKey()

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, sender.PublicKey().String(), []string{"contract-reader-interface"})
	rpcClient := rpc.New(url)

	utils.FundAccounts(t, []solana.PrivateKey{sender}, rpcClient)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, nil)
	require.NoError(t, err)

	multiClient := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return solanaClient, nil
	})

	t.Run("returns no instructions when no ATA location is found", func(t *testing.T) {
		lookups := []chainwriter.ATALookup{
			{
				Location: "Invalid.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: feePayer.String(),
				}},
				TokenProgram: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: solana.Token2022ProgramID.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Invalid.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: solanautils.GetRandomPubKey(t).Bytes()},
			},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
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
				TokenProgram: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: solana.Token2022ProgramID.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: solanautils.GetRandomPubKey(t).String(),
				}},
			},
		}

		args := map[string][]solana.PublicKey{
			"Addresses": {solanautils.GetRandomPubKey(t), solanautils.GetRandomPubKey(t)},
		}

		_, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.Contains(t, err.Error(), "expected exactly one wallet address, got 2")
	})

	t.Run("fails with mismatched mint and token programs", func(t *testing.T) {
		lookups := []chainwriter.ATALookup{
			{
				Location: "",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: feePayer.String(),
				}},
				TokenProgram: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: solana.Token2022ProgramID.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Addresses",
				}},
			},
		}

		args := map[string][]solana.PublicKey{
			"Addresses": {solanautils.GetRandomPubKey(t), solanautils.GetRandomPubKey(t)},
		}

		_, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.Contains(t, err.Error(), "expected equal number of token programs and mints, got 1 tokenPrograms and 2 mints")
	})

	t.Run("fails when mint is not a token address", func(t *testing.T) {
		tokenProgram := solana.Token2022ProgramID
		mint := solanautils.GetRandomPubKey(t)

		ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, feePayer)
		require.NoError(t, err)
		require.False(t, checkIfATAExists(t, rpcClient, ataAddress))
		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: feePayer.String(),
				}},
				TokenProgram: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: tokenProgram.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: mint.Bytes()},
			},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.NoError(t, err)

		tx := solanautils.CreateTx(ctx, t, rpcClient, ataInstructions, sender, rpc.CommitmentFinalized)

		_, err = rpcClient.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{SkipPreflight: false, PreflightCommitment: rpc.CommitmentProcessed})
		require.Contains(t, err.Error(), "Program log: Error: Invalid Mint")
	})

	t.Run("successfully creates ATAs only when necessary", func(t *testing.T) {
		tokenProgram := solana.Token2022ProgramID
		mint := utils.CreateRandomToken(t.Context(), t, sender, solana.Token2022ProgramID, rpcClient)

		ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, feePayer)
		require.NoError(t, err)
		require.False(t, checkIfATAExists(t, rpcClient, ataAddress))
		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: feePayer.String(),
				}},
				TokenProgram: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: tokenProgram.String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.Address",
				}},
			},
		}

		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: mint.Bytes()},
			},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.NoError(t, err)

		solanautils.SendAndConfirm(ctx, t, rpcClient, ataInstructions, sender, rpc.CommitmentFinalized)
		require.True(t, checkIfATAExists(t, rpcClient, ataAddress))

		// now, if we try to create the same ATA again, it should return no instructions
		ataInstructions, err = chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.NoError(t, err)
		require.Empty(t, ataInstructions)
	})

	t.Run("successfully creates multiple ATAs when necessary", func(t *testing.T) {
		tokenProgram := solana.Token2022ProgramID

		const numMints = 3
		var mints []solana.PublicKey
		for i := 0; i < numMints; i++ {
			mintPubKey := utils.CreateRandomToken(t.Context(), t, sender, tokenProgram, rpcClient)
			mints = append(mints, mintPubKey)
		}

		var ataAddresses []solana.PublicKey
		for _, mint := range mints {
			ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, feePayer)
			require.NoError(t, err)
			require.False(t, checkIfATAExists(t, rpcClient, ataAddress), "ATA should not exist yet")
			ataAddresses = append(ataAddresses, ataAddress)
		}

		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: feePayer.String(),
				}},
				TokenProgram: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.SecondAddress",
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

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.NoError(t, err)
		require.Len(t, ataInstructions, numMints)

		solanautils.SendAndConfirm(ctx, t, rpcClient, ataInstructions, sender, rpc.CommitmentFinalized)

		for _, ataAddress := range ataAddresses {
			require.True(t, checkIfATAExists(t, rpcClient, ataAddress), "ATA should have been created")
		}

		ataInstructions, err = chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.NoError(t, err)
		require.Empty(t, ataInstructions, "No new instructions should be returned if ATAs already exist")
	})

	t.Run("optional ATA creation does not return error if lookups fail", func(t *testing.T) {
		lookups := []chainwriter.ATALookup{
			{
				Location: "Inner.Address",
				WalletAddress: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: feePayer.String(),
				}},
				TokenProgram: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Address: solanautils.GetRandomPubKey(t).String(),
				}},
				MintAddress: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
					Location: "Inner.BadLocation",
				}},
				Optional: true,
			},
		}
		args := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{{Address: solanautils.GetRandomPubKey(t).Bytes()}},
		}

		ataInstructions, err := chainwriter.CreateATAs(ctx, args, lookups, nil, multiClient, feePayer, logger.Test(t))
		require.NoError(t, err)
		require.Len(t, ataInstructions, 0)
	})
}

func checkIfATAExists(t *testing.T, rpcClient *rpc.Client, ataAddress solana.PublicKey) bool {
	_, err := rpcClient.GetAccountInfo(t.Context(), ataAddress)
	return err == nil
}
