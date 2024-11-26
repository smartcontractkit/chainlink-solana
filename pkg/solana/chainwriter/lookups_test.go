package chainwriter_test

import (
	"context"
	"crypto/sha256"
	"reflect"
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

type DataAccount struct {
	Discriminator        [8]byte
	Version              uint8
	Administrator        solana.PublicKey
	PendingAdministrator solana.PublicKey
	LookupTable          solana.PublicKey
}

func TestAccountContant(t *testing.T) {
	t.Run("AccountConstant resolves valid address", func(t *testing.T) {
		expectedAddr := getRandomPubKey(t)
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
		result, err := constantConfig.Resolve(nil, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}
func TestAccountLookups(t *testing.T) {
	t.Run("AccountLookup resolves valid address with just one address", func(t *testing.T) {
		expectedAddr := getRandomPubKey(t)
		testArgs := TestArgs{
			Inner: []InnerArgs{
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
		result, err := lookupConfig.Resolve(nil, testArgs, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("AccountLookup resolves valid address with just multiple addresses", func(t *testing.T) {
		expectedAddr1 := getRandomPubKey(t)
		expectedAddr2 := getRandomPubKey(t)

		testArgs := TestArgs{
			Inner: []InnerArgs{
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
		result, err := lookupConfig.Resolve(nil, testArgs, nil, nil)
		require.NoError(t, err)
		for i, meta := range result {
			require.Equal(t, expectedMeta[i], meta)
		}
	})

	t.Run("AccountLookup fails when address isn't in args", func(t *testing.T) {
		expectedAddr := getRandomPubKey(t)

		testArgs := TestArgs{
			Inner: []InnerArgs{
				{Address: expectedAddr.Bytes()},
			},
		}
		lookupConfig := chainwriter.AccountLookup{
			Name:       "InvalidAccount",
			Location:   "Invalid.Directory",
			IsSigner:   true,
			IsWritable: true,
		}
		_, err := lookupConfig.Resolve(nil, testArgs, nil, nil)
		require.Error(t, err)
	})
}

func TestPDALookups(t *testing.T) {
	programID := solana.SystemProgramID

	t.Run("PDALookup resolves valid PDA with constant address seeds", func(t *testing.T) {
		seed := getRandomPubKey(t)

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

		result, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})

	t.Run("PDALookup fails with missing seeds", func(t *testing.T) {
		programID := solana.SystemProgramID

		pdaLookup := chainwriter.PDALookups{
			Name:      "TestPDA",
			PublicKey: chainwriter.AccountConstant{Name: "ProgramID", Address: programID.String()},
			Seeds: []chainwriter.Lookup{
				chainwriter.AccountLookup{Name: "seed1", Location: "MissingSeed"},
			},
			IsSigner:   false,
			IsWritable: true,
		}

		ctx := context.Background()
		args := map[string]interface{}{
			"test_seed": []byte("data"),
		}

		_, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "key not found")
	})

	t.Run("PDALookup resolves valid PDA with address lookup seeds", func(t *testing.T) {
		seed1 := getRandomPubKey(t)
		seed2 := getRandomPubKey(t)

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

		result, err := pdaLookup.Resolve(ctx, args, nil, nil)
		require.NoError(t, err)
		require.Equal(t, expectedMeta, result)
	})
}

func TestLookupTables(t *testing.T) {
	ctx := tests.Context(t)

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	url := utils.SetupTestValidatorWithAnchorPrograms(t, utils.PathToAnchorConfig, sender.PublicKey().String())
	rpcClient := rpc.New(url)

	utils.FundAccounts(ctx, []solana.PrivateKey{sender}, rpcClient, t)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, nil)
	require.NoError(t, err)

	loader := commonutils.NewLazyLoad(func() (client.ReaderWriter, error) { return solanaClient, nil })
	mkey := keyMocks.NewSimpleKeystore(t)
	lggr := logger.Test(t)

	txm := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)

	cw, err := chainwriter.NewSolanaChainWriterService(solanaClient, *txm, nil, chainwriter.ChainWriterConfig{})

	t.Run("StaticLookup table resolves properly", func(t *testing.T) {
		pubKeys := createTestPubKeys(t, 8)
		table := CreateTestLookupTable(ctx, t, rpcClient, sender, pubKeys)
		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: nil,
			StaticLookupTables:  []string{table.String()},
		}
		_, staticTableMap, err := cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.NoError(t, err)
		require.Equal(t, pubKeys, staticTableMap[table])
	})
	t.Run("Derived lookup table resolves properly with constant address", func(t *testing.T) {
		pubKeys := createTestPubKeys(t, 8)
		table := CreateTestLookupTable(ctx, t, rpcClient, sender, pubKeys)
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
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.NoError(t, err)

		addresses, ok := derivedTableMap["DerivedTable"][table.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})

	t.Run("Derived lookup table fails with invalid address", func(t *testing.T) {
		invalidTable := getRandomPubKey(t)

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
		invalidTable := getRandomPubKey(t)

		lookupConfig := chainwriter.LookupTables{
			DerivedLookupTables: nil,
			StaticLookupTables:  []string{invalidTable.String()},
		}

		_, _, err = cw.ResolveLookupTables(ctx, nil, lookupConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error fetching account info for table") // Example error message
	})

	t.Run("Derived lookup table resolves properly with account lookup address", func(t *testing.T) {
		pubKeys := createTestPubKeys(t, 8)
		table := CreateTestLookupTable(ctx, t, rpcClient, sender, pubKeys)
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

		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, testArgs, lookupConfig)
		require.NoError(t, err)

		addresses, ok := derivedTableMap["DerivedTable"][table.String()]
		require.True(t, ok)
		for i, address := range addresses {
			require.Equal(t, pubKeys[i], address.PublicKey)
		}
	})

	t.Run("Derived lookup table resolves properly with PDALookup address", func(t *testing.T) {
		// Deployed write_test contract
		programID := solana.MustPublicKeyFromBase58("39vbQVpEMtZtg3e6ZSE7nBSzmNZptmW45WnLkbqEe4TU")

		lookupKeys := createTestPubKeys(t, 5)
		lookupTable := CreateTestLookupTable(ctx, t, rpcClient, sender, lookupKeys)

		InitializeDataAccount(ctx, t, rpcClient, programID, sender, lookupTable)

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
						Seeds: []chainwriter.Lookup{
							chainwriter.AccountLookup{Name: "seed1", Location: "seed1"},
						},
						IsSigner:   false,
						IsWritable: false,
						InternalField: chainwriter.InternalField{
							Type:     reflect.TypeOf(DataAccount{}),
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

func InitializeDataAccount(
	ctx context.Context,
	t *testing.T,
	client *rpc.Client,
	programID solana.PublicKey,
	admin solana.PrivateKey,
	lookupTable solana.PublicKey,
) {
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("data")}, programID)
	require.NoError(t, err)

	discriminator := getDiscriminator("initialize")

	instructionData := append(discriminator[:], lookupTable.Bytes()...)

	instruction := solana.NewInstruction(
		programID,
		solana.AccountMetaSlice{
			solana.Meta(pda).WRITE(),
			solana.Meta(admin.PublicKey()).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		instructionData,
	)

	// Send and confirm the transaction
	utils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, rpc.CommitmentFinalized)
}

func getDiscriminator(instruction string) [8]byte {
	fullHash := sha256.Sum256([]byte("global:" + instruction))
	var discriminator [8]byte
	copy(discriminator[:], fullHash[:8])
	return discriminator
}

func getRandomPubKey(t *testing.T) solana.PublicKey {
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey()
}

func createTestPubKeys(t *testing.T, num int) solana.PublicKeySlice {
	addresses := make([]solana.PublicKey, num)
	for i := 0; i < num; i++ {
		addresses[i] = getRandomPubKey(t)
	}
	return addresses
}

func CreateTestLookupTable(ctx context.Context, t *testing.T, c *rpc.Client, sender solana.PrivateKey, addresses []solana.PublicKey) solana.PublicKey {
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
