package chainwriter_test

import (
	"context"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"

	commonutils "github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/test-go/testify/require"
)

type TestArgs struct {
	Inner []InnerArgs
}

type InnerArgs struct {
	Address []byte
}

func TestAccountContant(t *testing.T) {
	t.Run("AccountConstant resolves valid address", func(t *testing.T) {
		expectedAddr := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M"
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  solana.MustPublicKeyFromBase58(expectedAddr),
				IsSigner:   true,
				IsWritable: true,
			},
		}
		constantConfig := chainwriter.AccountConstant{
			Name:       "TestAccount",
			Address:    expectedAddr,
			IsSigner:   true,
			IsWritable: true,
		}
		result, err := constantConfig.Resolve(nil, nil, nil, "")
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}
func TestAccountLookups(t *testing.T) {
	t.Run("AccountLookup resolves valid address with just one address", func(t *testing.T) {
		expectedAddr := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M"
		testArgs := TestArgs{
			Inner: []InnerArgs{
				{Address: solana.MustPublicKeyFromBase58(expectedAddr).Bytes()},
			},
		}
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  solana.MustPublicKeyFromBase58(expectedAddr),
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
		result, err := lookupConfig.Resolve(nil, testArgs, nil, "")
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("AccountLookup resolves valid address with just multiple addresses", func(t *testing.T) {
		expectedAddr1 := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M"
		expectedAddr2 := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6N"
		testArgs := TestArgs{
			Inner: []InnerArgs{
				{Address: solana.MustPublicKeyFromBase58(expectedAddr1).Bytes()},
				{Address: solana.MustPublicKeyFromBase58(expectedAddr2).Bytes()},
			},
		}
		expectedMeta := []*solana.AccountMeta{
			{
				PublicKey:  solana.MustPublicKeyFromBase58(expectedAddr1),
				IsSigner:   true,
				IsWritable: true,
			},
			{
				PublicKey:  solana.MustPublicKeyFromBase58(expectedAddr2),
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
		result, err := lookupConfig.Resolve(nil, testArgs, nil, "")
		require.NoError(t, err)
		for i, meta := range result {
			require.Equal(t, expectedMeta[i], meta)
		}
	})

	t.Run("AccountLookup fails when address isn't in args", func(t *testing.T) {
		expectedAddr := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M"
		testArgs := TestArgs{
			Inner: []InnerArgs{
				{Address: solana.MustPublicKeyFromBase58(expectedAddr).Bytes()},
			},
		}
		lookupConfig := chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Invalid.Directory",
			IsSigner:   true,
			IsWritable: true,
		}
		_, err := lookupConfig.Resolve(nil, testArgs, nil, "")
		require.Error(t, err)
	})
}

func TestPDALookups(t *testing.T) {
	programID := solana.SystemProgramID

	t.Run("PDALookup resolves valid PDA with constant address seeds", func(t *testing.T) {
		privKey, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		seed := privKey.PublicKey()

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
			Seeds: []chainwriter.Lookup{
				chainwriter.AccountConstant{Name: "seed", Address: seed.String()},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		ctx := context.Background()
		result, err := pdaLookup.Resolve(ctx, nil, nil, "")
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
			Seeds: []chainwriter.Lookup{
				chainwriter.AccountLookup{Name: "seed1", Location: "test_seed"},
				chainwriter.AccountLookup{Name: "seed2", Location: "another_seed"},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		ctx := context.Background()
		args := map[string]interface{}{
			"test_seed":    seed1,
			"another_seed": seed2,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, "")
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookup resolves valid PDA with address lookup seeds", func(t *testing.T) {
		privKey1, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		seed1 := privKey1.PublicKey()

		privKey2, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		seed2 := privKey2.PublicKey()

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
			Seeds: []chainwriter.Lookup{
				chainwriter.AccountLookup{Name: "seed1", Location: "test_seed"},
				chainwriter.AccountLookup{Name: "seed2", Location: "another_seed"},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		ctx := context.Background()
		args := map[string]interface{}{
			"test_seed":    seed1,
			"another_seed": seed2,
		}

		result, err := pdaLookup.Resolve(ctx, args, nil, "")
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}

func TestLookupTables(t *testing.T) {
	ctx := tests.Context(t)
	url := client.SetupLocalSolNode(t)
	c := rpc.New(url)

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	utils.FundAccounts(ctx, []solana.PrivateKey{sender}, c, t)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, nil)

	loader := commonutils.NewLazyLoad(func() (client.ReaderWriter, error) { return solanaClient, nil })
	mkey := keyMocks.NewSimpleKeystore(t)
	lggr := logger.Test(t)

	txm := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)

	chainWriter, err := chainwriter.NewSolanaChainWriterService(solanaClient, *txm, nil, chainwriter.ChainWriterConfig{})

	t.Run("StaticLookup table resolves properly", func(t *testing.T) {
		pubKeys := createTestPubKeys(t, 8)
		table := CreateTestLookupTable(t, ctx, c, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: nil,
			StaticLookupTables:  []string{table.String()},
		}
		_, staticTableMap, err := chainWriter.ResolveLookupTables(ctx, nil, lookupConfig, "test-debug-id")
		require.NoError(t, err)
		require.Equal(t, pubKeys, staticTableMap[table])
	})
	t.Run("Derived lookup table resovles properly with constant address", func(t *testing.T) {
		pubKeys := createTestPubKeys(t, 8)
		table := CreateTestLookupTable(t, ctx, c, sender, pubKeys)
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
		derivedTableMap, _, err := chainWriter.ResolveLookupTables(ctx, nil, lookupConfig, "test-debug-id")
		require.NoError(t, err)

		addresses, ok := derivedTableMap["DerivedTable"][table.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})

	t.Run("Derived lookup table resolves properly with account lookup address", func(t *testing.T) {
		pubKeys := createTestPubKeys(t, 8)
		table := CreateTestLookupTable(t, ctx, c, sender, pubKeys)
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

		testArgs := TestArgs{
			Inner: []InnerArgs{
				{Address: table.Bytes()},
			},
		}

		derivedTableMap, _, err := chainWriter.ResolveLookupTables(ctx, testArgs, lookupConfig, "test-debug-id")
		require.NoError(t, err)

		addresses, ok := derivedTableMap["DerivedTable"][table.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})
}

func createTestPubKeys(t *testing.T, num int) solana.PublicKeySlice {
	addresses := make([]solana.PublicKey, num)
	for i := 0; i < num; i++ {
		privKey, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		addresses[i] = privKey.PublicKey()
	}
	return addresses
}

func CreateTestLookupTable(t *testing.T, ctx context.Context, c *rpc.Client, sender solana.PrivateKey, addresses []solana.PublicKey) solana.PublicKey {
	// Create lookup tables
	slot, serr := c.GetSlot(ctx, rpc.CommitmentFinalized)
	require.NoError(t, serr)
	table, instruction, ierr := utils.NewCreateLookupTableInstruction(
		sender.PublicKey(),
		sender.PublicKey(),
		slot,
	)
	require.NoError(t, ierr)
	utils.SendAndConfirm(ctx, t, c, []solana.Instruction{instruction}, sender, rpc.CommitmentConfirmed)

	// add entries to lookup table
	utils.SendAndConfirm(ctx, t, c, []solana.Instruction{
		utils.NewExtendLookupTableInstruction(
			table, sender.PublicKey(), sender.PublicKey(),
			addresses,
		),
	}, sender, rpc.CommitmentConfirmed)

	return table
}
