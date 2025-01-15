package relayinterface

import (
	"reflect"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonutils "github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

func TestAccountContant(t *testing.T) {
	t.Run("AccountConstant resolves valid address", func(t *testing.T) {
		expectedAddr := chainwriter.GetRandomPubKey(t)
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  expectedAddr,
				IsSigner:   true,
				IsWritable: true,
			},
		}
		constantConfig := chainwriter.AccountConstant{
			Name:       "TestAccount",
			Address:    expectedAddr.String(),
			IsSigner:   true,
			IsWritable: true,
		}
		result, err := constantConfig.Resolve(tests.Context(t), nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}
func TestAccountLookups(t *testing.T) {
	ctx := tests.Context(t)
	t.Run("AccountLookup resolves valid address with just one address", func(t *testing.T) {
		expectedAddr := chainwriter.GetRandomPubKey(t)
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

		lookupConfig := chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Inner.Address",
			IsSigner:   true,
			IsWritable: true,
		}
		result, err := lookupConfig.Resolve(ctx, testArgs, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("AccountLookup resolves valid address with just multiple addresses", func(t *testing.T) {
		expectedAddr1 := chainwriter.GetRandomPubKey(t)
		expectedAddr2 := chainwriter.GetRandomPubKey(t)

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

		lookupConfig := chainwriter.AccountLookup{
			Name:       "TestAccount",
			Location:   "Inner.Address",
			IsSigner:   true,
			IsWritable: true,
		}
		result, err := lookupConfig.Resolve(ctx, testArgs, nil, nil)
		require.NoError(t, err)
		for i, meta := range result {
			require.Equal(t, expectedMeta[i], meta)
		}
	})

	t.Run("AccountLookup fails when address isn't in args", func(t *testing.T) {
		expectedAddr := chainwriter.GetRandomPubKey(t)

		testArgs := chainwriter.TestArgs{
			Inner: []chainwriter.InnerArgs{
				{Address: expectedAddr.Bytes()},
			},
		}
		lookupConfig := chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Invalid.Directory",
			IsSigner:   true,
			IsWritable: true,
		}
		_, err := lookupConfig.Resolve(ctx, testArgs, nil, nil)
		require.Error(t, err)
	})
}

func TestPDALookups(t *testing.T) {
	programID := chainwriter.GetRandomPubKey(t)
	ctx := tests.Context(t)

	t.Run("PDALookup resolves valid PDA with constant address seeds", func(t *testing.T) {
		seed := chainwriter.GetRandomPubKey(t)

		pda, _, err := solana.FindProgramAddress([][]byte{seed.Bytes()}, programID)
		require.NoError(t, err)

		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: true,
			},
		}

		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.AccountConstant{Name: "seed", Address: seed.String()}},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		result, err := pdaLookup.Resolve(ctx, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
	t.Run("PDALookup resolves valid PDA with non-address lookup seeds", func(t *testing.T) {
		seed1 := []byte("test_seed")
		seed2 := []byte("another_seed")

		pda, _, err := solana.FindProgramAddress([][]byte{seed1, seed2}, programID)
		require.NoError(t, err)

		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: true,
			},
		}

		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.AccountLookup{Name: "seed1", Location: "test_seed"}},
				{Dynamic: chainwriter.AccountLookup{Name: "seed2", Location: "another_seed"}},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		args := map[string]interface{}{
			"test_seed":    seed1,
			"another_seed": seed2,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookup fails with missing seeds", func(t *testing.T) {
		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.AccountLookup{Name: "seed1", Location: "MissingSeed"}},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		args := map[string]interface{}{
			"test_seed": []byte("data"),
		}

		_, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "key not found")
	})

	t.Run("PDALookup resolves valid PDA with address lookup seeds", func(t *testing.T) {
		seed1 := chainwriter.GetRandomPubKey(t)
		seed2 := chainwriter.GetRandomPubKey(t)

		pda, _, err := solana.FindProgramAddress([][]byte{seed1.Bytes(), seed2.Bytes()}, programID)
		require.NoError(t, err)

		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  pda,
				IsSigner:   false,
				IsWritable: true,
			},
		}

		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.AccountLookup{Name: "seed1", Location: "test_seed"}},
				{Dynamic: chainwriter.AccountLookup{Name: "seed2", Location: "another_seed"}},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		args := map[string]interface{}{
			"test_seed":    seed1,
			"another_seed": seed2,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookups resolves list of PDAs when a seed is an array", func(t *testing.T) {
		singleSeed := []byte("test_seed")
		arraySeed := []solana.PublicKey{chainwriter.GetRandomPubKey(t), chainwriter.GetRandomPubKey(t)}

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

		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.AccountLookup{Name: "seed1", Location: "single_seed"}},
				{Dynamic: chainwriter.AccountLookup{Name: "seed2", Location: "array_seed"}},
			},
			IsSigner:   false,
			IsWritable: false,
		}

		args := map[string]interface{}{
			"single_seed": singleSeed,
			"array_seed":  arraySeed,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookups resolves list of PDAs when multiple seeds are arrays", func(t *testing.T) {
		arraySeed1 := [][]byte{[]byte("test_seed1"), []byte("test_seed2")}
		arraySeed2 := []solana.PublicKey{chainwriter.GetRandomPubKey(t), chainwriter.GetRandomPubKey(t)}

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

		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Seed{
				{Dynamic: chainwriter.AccountLookup{Name: "seed1", Location: "seed1"}},
				{Dynamic: chainwriter.AccountLookup{Name: "seed2", Location: "seed2"}},
			},
			IsSigner:   false,
			IsWritable: false,
		}

		args := map[string]interface{}{
			"seed1": arraySeed1,
			"seed2": arraySeed2,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}

func TestLookupTables(t *testing.T) {
	ctx := tests.Context(t)

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	url, _ := utils.SetupTestValidatorWithAnchorPrograms(t, sender.PublicKey().String(), []string{"contract-reader-interface"})
	rpcClient := rpc.New(url)

	utils.FundAccounts(t, []solana.PrivateKey{sender}, rpcClient)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, nil)
	require.NoError(t, err)

	loader := commonutils.NewLazyLoad(func() (client.ReaderWriter, error) { return solanaClient, nil })
	mkey := keyMocks.NewSimpleKeystore(t)
	lggr := logger.Test(t)

	txm := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)

	cw, err := chainwriter.NewSolanaChainWriterService(nil, solanaClient, txm, nil, chainwriter.ChainWriterConfig{})

	t.Run("StaticLookup table resolves properly", func(t *testing.T) {
		pubKeys := chainwriter.CreateTestPubKeys(t, 8)
		table := chainwriter.CreateTestLookupTable(ctx, t, rpcClient, sender, pubKeys)
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
		table := chainwriter.CreateTestLookupTable(ctx, t, rpcClient, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.AccountConstant{
						Name:       "TestLookupTable",
						Address:    table.String(),
						IsSigner:   true,
						IsWritable: true,
					},
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
		invalidTable := chainwriter.GetRandomPubKey(t)

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.AccountConstant{
						Name:       "InvalidTable",
						Address:    invalidTable.String(),
						IsSigner:   true,
						IsWritable: true,
					},
				},
			},
			StaticLookupTables: nil,
		}

		_, _, err = cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error fetching account info for table") // Example error message
	})

	t.Run("Static lookup table fails with invalid address", func(t *testing.T) {
		invalidTable := chainwriter.GetRandomPubKey(t)

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
		table := chainwriter.CreateTestLookupTable(ctx, t, rpcClient, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.AccountLookup{
						Name:     "TestLookupTable",
						Location: "Inner.Address",
						IsSigner: true,
					},
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

		addresses, ok := derivedTableMap["DerivedTable"][table.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})

	t.Run("Derived lookup table resolves properly with PDALookup address", func(t *testing.T) {
		// Deployed contract_reader_interface contract
		programID := solana.MustPublicKeyFromBase58("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE")

		lookupKeys := chainwriter.CreateTestPubKeys(t, 5)
		lookupTable := chainwriter.CreateTestLookupTable(ctx, t, rpcClient, sender, lookupKeys)

		chainwriter.InitializeDataAccount(ctx, t, rpcClient, programID, sender, lookupTable)

		args := map[string]interface{}{
			"seed1": []byte("data"),
		}

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "DerivedTable",
					Accounts: chainwriter.PDALookups{
						Name:      "DataAccountPDA",
						PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()},
						Seeds: []chainwriter.Seed{
							{Dynamic: chainwriter.AccountLookup{Name: "seed1", Location: "seed1"}},
						},
						IsSigner:   false,
						IsWritable: false,
						InternalField: chainwriter.InternalField{
							Type:     reflect.TypeOf(chainwriter.DataAccount{}),
							Location: "LookupTable",
						},
					},
				},
			},
			StaticLookupTables: nil,
		}

		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupConfig)
		require.NoError(t, err)

		addresses, ok := derivedTableMap["DerivedTable"][lookupTable.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, lookupKeys[i], address.PublicKey)
		}
	})
}
