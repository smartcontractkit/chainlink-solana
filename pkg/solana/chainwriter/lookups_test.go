package chainwriter_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"encoding/binary"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"

	"github.com/smartcontractkit/chainlink-common/pkg/utils"
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
	// TODO: May require deploying a program to test
	// t.Run("PDALookup resolves valid address", func(t *testing.T) {
	// 	expectedAddr := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M"
	// 	expectedMeta := []*solana.AccountMeta{
	// 		{
	// 			PublicKey:  solana.MustPublicKeyFromBase58(expectedAddr),
	// 			IsSigner:   true,
	// 			IsWritable: true,
	// 		},
	// 	}
	// 	lookupConfig := chainwriter.PDALookups{
	// 		Name: "TestAccount",
	// 		PublicKey:
	// 	}

	// })
}

func TestLookupTables(t *testing.T) {
	ctx := tests.Context(t)
	url := client.SetupLocalSolNode(t)
	c := rpc.New(url)

	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	client.FundTestAccounts(t, []solana.PublicKey{sender.PublicKey()}, url)

	cfg := config.NewDefault()
	solanaClient, err := client.NewClient(url, cfg, 5*time.Second, nil)

	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return solanaClient, nil })
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
		_, staticTableMap, err := chainWriter.ResolveLookupTables(ctx, lookupConfig, "test-debug-id")
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
		derivedTableMap, _, err := chainWriter.ResolveLookupTables(ctx, lookupConfig, "test-debug-id")
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
	fmt.Println("SLOT: ", slot)
	require.NoError(t, serr)
	table, instruction, ierr := NewCreateLookupTableInstruction(
		sender.PublicKey(),
		sender.PublicKey(),
		slot,
	)
	require.NoError(t, ierr)
	SendAndConfirm(ctx, t, c, []solana.Instruction{instruction}, sender, rpc.CommitmentConfirmed)

	// add entries to lookup table
	SendAndConfirm(ctx, t, c, []solana.Instruction{
		NewExtendLookupTableInstruction(
			table, sender.PublicKey(), sender.PublicKey(),
			addresses,
		),
	}, sender, rpc.CommitmentConfirmed)

	return table
}

// TxModifier is a dynamic function used to flexibly add components to a transaction such as additional signers, and compute budget parameters
type TxModifier func(tx *solana.Transaction, signers map[solana.PublicKey]solana.PrivateKey) error

func SendAndConfirm(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, opts ...TxModifier) *rpc.GetTransactionResult {
	txres := sendTransaction(ctx, rpcClient, t, instructions, signer, commitment, false, opts...) // do not skipPreflight when expected to pass, preflight can help debug

	require.NotNil(t, txres.Meta)
	require.Nil(t, txres.Meta.Err, fmt.Sprintf("tx failed with: %+v", txres.Meta)) // tx should not err, print meta if it does (contains logs)
	return txres
}

func sendTransaction(ctx context.Context, rpcClient *rpc.Client, t *testing.T, instructions []solana.Instruction,
	signerAndPayer solana.PrivateKey, commitment rpc.CommitmentType, skipPreflight bool, opts ...TxModifier) *rpc.GetTransactionResult {
	hashRes, err := rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	require.NoError(t, err)

	tx, err := solana.NewTransaction(
		instructions,
		hashRes.Value.Blockhash,
		solana.TransactionPayer(signerAndPayer.PublicKey()),
	)
	require.NoError(t, err)

	// build signers map
	signers := map[solana.PublicKey]solana.PrivateKey{}
	signers[signerAndPayer.PublicKey()] = signerAndPayer

	// set options before signing transaction
	for _, o := range opts {
		require.NoError(t, o(tx, signers))
	}

	_, err = tx.Sign(func(pub solana.PublicKey) *solana.PrivateKey {
		priv, ok := signers[pub]
		require.True(t, ok, fmt.Sprintf("Missing signer private key for %s", pub))
		return &priv
	})
	require.NoError(t, err)

	txsig, err := rpcClient.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{SkipPreflight: skipPreflight, PreflightCommitment: rpc.CommitmentProcessed})
	require.NoError(t, err)

	var txStatus rpc.ConfirmationStatusType
	count := 0
	for txStatus != rpc.ConfirmationStatusConfirmed && txStatus != rpc.ConfirmationStatusFinalized {
		count++
		statusRes, sigErr := rpcClient.GetSignatureStatuses(ctx, true, txsig)
		require.NoError(t, sigErr)
		if statusRes != nil && len(statusRes.Value) > 0 && statusRes.Value[0] != nil {
			txStatus = statusRes.Value[0].ConfirmationStatus
		}
		time.Sleep(100 * time.Millisecond)
		if count > 50 {
			require.NoError(t, fmt.Errorf("unable to find transaction within timeout"))
		}
	}

	txres, err := rpcClient.GetTransaction(ctx, txsig, &rpc.GetTransactionOpts{
		Commitment: commitment,
	})
	require.NoError(t, err)
	return txres
}

var (
	AddressLookupTableProgram = solana.MustPublicKeyFromBase58("AddressLookupTab1e1111111111111111111111111")
)

const (
	InstructionCreateLookupTable uint32 = iota
	InstructionFreezeLookupTable
	InstructionExtendLookupTable
	InstructionDeactiveLookupTable
	InstructionCloseLookupTable
)

func NewCreateLookupTableInstruction(
	authority, funder solana.PublicKey,
	slot uint64,
) (solana.PublicKey, solana.Instruction, error) {
	// https://github.com/solana-labs/solana-web3.js/blob/c1c98715b0c7900ce37c59bffd2056fa0037213d/src/programs/address-lookup-table/index.ts#L274
	slotLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotLE, slot)
	account, bumpSeed, err := solana.FindProgramAddress([][]byte{authority.Bytes(), slotLE}, AddressLookupTableProgram)
	if err != nil {
		return solana.PublicKey{}, nil, err
	}

	data := binary.LittleEndian.AppendUint32([]byte{}, InstructionCreateLookupTable)
	data = binary.LittleEndian.AppendUint64(data, slot)
	data = append(data, bumpSeed)
	return account, solana.NewInstruction(
		AddressLookupTableProgram,
		solana.AccountMetaSlice{
			solana.Meta(account).WRITE(),
			solana.Meta(authority).SIGNER(),
			solana.Meta(funder).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		data,
	), nil
}

func NewExtendLookupTableInstruction(
	table, authority, funder solana.PublicKey,
	accounts []solana.PublicKey,
) solana.Instruction {
	// https://github.com/solana-labs/solana-web3.js/blob/c1c98715b0c7900ce37c59bffd2056fa0037213d/src/programs/address-lookup-table/index.ts#L113

	data := binary.LittleEndian.AppendUint32([]byte{}, InstructionExtendLookupTable)
	data = binary.LittleEndian.AppendUint64(data, uint64(len(accounts))) // note: this is usually u32 + 8 byte buffer
	for _, a := range accounts {
		data = append(data, a.Bytes()...)
	}

	return solana.NewInstruction(
		AddressLookupTableProgram,
		solana.AccountMetaSlice{
			solana.Meta(table).WRITE(),
			solana.Meta(authority).SIGNER(),
			solana.Meta(funder).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		data,
	)
}
