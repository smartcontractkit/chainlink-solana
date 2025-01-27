package chainwriter_test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	idl "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	feemocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/fees/mocks"
	txmMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
)

type Arguments struct {
	LookupTable solana.PublicKey
	Seed1       []byte
	Seed2       []byte
}

var ccipRouterIDL = idl.FetchCCIPRouterIDL()
var testContractIDL = chainwriter.FetchTestContractIDL()

func TestChainWriter_GetAddresses(t *testing.T) {
	ctx := tests.Context(t)

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	// expected account meta for constant account
	constantAccountMeta := &solana.AccountMeta{
		IsSigner:   true,
		IsWritable: true,
	}

	// expected account meta for account lookup
	accountLookupMeta := &solana.AccountMeta{
		IsSigner:   true,
		IsWritable: false,
	}

	// setup pda account address
	seed1 := []byte("seed1")
	pda1 := mustFindPdaProgramAddress(t, [][]byte{seed1}, solana.SystemProgramID)
	// expected account meta for pda lookup
	pdaLookupMeta := &solana.AccountMeta{
		PublicKey:  pda1,
		IsSigner:   false,
		IsWritable: false,
	}

	// setup pda account with inner field lookup
	programID := chainwriter.GetRandomPubKey(t)
	seed2 := []byte("seed2")
	pda2 := mustFindPdaProgramAddress(t, [][]byte{seed2}, programID)
	// mock data account response from program
	lookupTablePubkey := mockDataAccountLookupTable(t, rw, pda2)
	// mock fetch lookup table addresses call
	storedPubKeys := chainwriter.CreateTestPubKeys(t, 3)
	mockFetchLookupTableAddresses(t, rw, lookupTablePubkey, storedPubKeys)
	// expected account meta for derived table lookup
	derivedTablePdaLookupMeta := &solana.AccountMeta{
		IsSigner:   false,
		IsWritable: true,
	}

	lookupTableConfig := chainwriter.LookupTables{
		DerivedLookupTables: []chainwriter.DerivedLookupTable{
			{
				Name: "DerivedTable",
				Accounts: chainwriter.PDALookups{
					Name:      "DataAccountPDA",
					PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()},
					Seeds: []chainwriter.Seed{
						// extract seed2 for PDA lookup
						{Dynamic: chainwriter.AccountLookup{Name: "Seed2", Location: "Seed2"}},
					},
					IsSigner:   derivedTablePdaLookupMeta.IsSigner,
					IsWritable: derivedTablePdaLookupMeta.IsWritable,
					InternalField: chainwriter.InternalField{
						TypeName: "LookupTableDataAccount",
						Location: "LookupTable",
					},
				},
			},
		},
		StaticLookupTables: nil,
	}

	t.Run("resolve addresses from different types of lookups", func(t *testing.T) {
		constantAccountMeta.PublicKey = chainwriter.GetRandomPubKey(t)
		accountLookupMeta.PublicKey = chainwriter.GetRandomPubKey(t)
		// correlates to DerivedTable index in account lookup config
		derivedTablePdaLookupMeta.PublicKey = storedPubKeys[0]

		args := Arguments{
			LookupTable: accountLookupMeta.PublicKey,
			Seed1:       seed1,
			Seed2:       seed2,
		}

		accountLookupConfig := []chainwriter.Lookup{
			chainwriter.AccountConstant{
				Name:       "Constant",
				Address:    constantAccountMeta.PublicKey.String(),
				IsSigner:   constantAccountMeta.IsSigner,
				IsWritable: constantAccountMeta.IsWritable,
			},
			chainwriter.AccountLookup{
				Name:       "LookupTable",
				Location:   "LookupTable",
				IsSigner:   chainwriter.MetaBool{Value: accountLookupMeta.IsSigner},
				IsWritable: chainwriter.MetaBool{Value: accountLookupMeta.IsWritable},
			},
			chainwriter.PDALookups{
				Name:      "DataAccountPDA",
				PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: solana.SystemProgramID.String()},
				Seeds: []chainwriter.Seed{
					// extract seed1 for PDA lookup
					{Dynamic: chainwriter.AccountLookup{Name: "Seed1", Location: "Seed1"}},
				},
				IsSigner:   pdaLookupMeta.IsSigner,
				IsWritable: pdaLookupMeta.IsWritable,
				// Just get the address of the account, nothing internal.
				InternalField: chainwriter.InternalField{},
			},
			chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0},
			},
		}

		// Fetch derived table map
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig, testContractIDL)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, rw, testContractIDL)
		require.NoError(t, err)

		// account metas should be returned in the same order as the provided account lookup configs
		require.Len(t, accounts, 4)

		// Validate account constant
		require.Equal(t, constantAccountMeta.PublicKey, accounts[0].PublicKey)
		require.Equal(t, constantAccountMeta.IsSigner, accounts[0].IsSigner)
		require.Equal(t, constantAccountMeta.IsWritable, accounts[0].IsWritable)

		// Validate account lookup
		require.Equal(t, accountLookupMeta.PublicKey, accounts[1].PublicKey)
		require.Equal(t, accountLookupMeta.IsSigner, accounts[1].IsSigner)
		require.Equal(t, accountLookupMeta.IsWritable, accounts[1].IsWritable)

		// Validate pda lookup
		require.Equal(t, pdaLookupMeta.PublicKey, accounts[2].PublicKey)
		require.Equal(t, pdaLookupMeta.IsSigner, accounts[2].IsSigner)
		require.Equal(t, pdaLookupMeta.IsWritable, accounts[2].IsWritable)

		// Validate pda lookup with inner field from derived table
		require.Equal(t, derivedTablePdaLookupMeta.PublicKey, accounts[3].PublicKey)
		require.Equal(t, derivedTablePdaLookupMeta.IsSigner, accounts[3].IsSigner)
		require.Equal(t, derivedTablePdaLookupMeta.IsWritable, accounts[3].IsWritable)
	})

	t.Run("resolve addresses for multiple indices from derived lookup table", func(t *testing.T) {
		args := Arguments{
			Seed2: seed2,
		}

		accountLookupConfig := []chainwriter.Lookup{
			chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0, 2},
			},
		}

		// Fetch derived table map
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig, testContractIDL)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, rw, testContractIDL)
		require.NoError(t, err)

		require.Len(t, accounts, 2)
		require.Equal(t, storedPubKeys[0], accounts[0].PublicKey)
		require.Equal(t, storedPubKeys[2], accounts[1].PublicKey)
	})

	t.Run("resolve all addresses from derived lookup table if indices not specified", func(t *testing.T) {
		args := Arguments{
			Seed2: seed2,
		}

		accountLookupConfig := []chainwriter.Lookup{
			chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
			},
		}

		// Fetch derived table map
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig, testContractIDL)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, rw, testContractIDL)
		require.NoError(t, err)

		require.Len(t, accounts, 3)
		for i, storedPubkey := range storedPubKeys {
			require.Equal(t, storedPubkey, accounts[i].PublicKey)
		}
	})
}

func TestChainWriter_FilterLookupTableAddresses(t *testing.T) {
	ctx := tests.Context(t)

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	programID := chainwriter.GetRandomPubKey(t)
	seed1 := []byte("seed1")
	pda1 := mustFindPdaProgramAddress(t, [][]byte{seed1}, programID)
	// mock data account response from program
	lookupTablePubkey := mockDataAccountLookupTable(t, rw, pda1)
	// mock fetch lookup table addresses call
	storedPubKey := chainwriter.GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, lookupTablePubkey, []solana.PublicKey{storedPubKey})

	unusedProgramID := chainwriter.GetRandomPubKey(t)
	seed2 := []byte("seed2")
	unusedPda := mustFindPdaProgramAddress(t, [][]byte{seed2}, unusedProgramID)
	// mock data account response from program
	unusedLookupTable := mockDataAccountLookupTable(t, rw, unusedPda)
	// mock fetch lookup table addresses call
	unusedKeys := chainwriter.GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, unusedLookupTable, []solana.PublicKey{unusedKeys})

	// mock static lookup table calls
	staticLookupTablePubkey1 := chainwriter.GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, staticLookupTablePubkey1, chainwriter.CreateTestPubKeys(t, 2))
	staticLookupTablePubkey2 := chainwriter.GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, staticLookupTablePubkey2, chainwriter.CreateTestPubKeys(t, 2))

	lookupTableConfig := chainwriter.LookupTables{
		DerivedLookupTables: []chainwriter.DerivedLookupTable{
			{
				Name: "DerivedTable",
				Accounts: chainwriter.PDALookups{
					Name:      "DataAccountPDA",
					PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()},
					Seeds: []chainwriter.Seed{
						// extract seed1 for PDA lookup
						{Dynamic: chainwriter.AccountLookup{Name: "Seed1", Location: "Seed1"}},
					},
					IsSigner:   true,
					IsWritable: true,
					InternalField: chainwriter.InternalField{
						TypeName: "LookupTableDataAccount",
						Location: "LookupTable",
					},
				},
			},
			{
				Name: "MiscDerivedTable",
				Accounts: chainwriter.PDALookups{
					Name:      "MiscPDA",
					PublicKey: chainwriter.AccountConstant{Name: "UnusedAccount", Address: unusedProgramID.String()},
					Seeds: []chainwriter.Seed{
						// extract seed2 for PDA lookup
						{Dynamic: chainwriter.AccountLookup{Name: "Seed2", Location: "Seed2"}},
					},
					IsSigner:   true,
					IsWritable: true,
					InternalField: chainwriter.InternalField{
						TypeName: "LookupTableDataAccount",
						Location: "LookupTable",
					},
				},
			},
		},
		StaticLookupTables: []solana.PublicKey{staticLookupTablePubkey1, staticLookupTablePubkey2},
	}

	args := Arguments{
		Seed1: seed1,
		Seed2: seed2,
	}

	t.Run("returns filtered map with only relevant addresses required by account lookup config", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{
			chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0},
			},
		}

		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig, testContractIDL)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, rw, testContractIDL)
		require.NoError(t, err)

		// Filter the lookup table addresses based on which accounts are actually used
		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)

		// Filter map should only contain the address for the DerivedTable lookup defined in the account lookup config
		require.Len(t, filteredLookupTableMap, len(accounts))
		entry, exists := filteredLookupTableMap[lookupTablePubkey]
		require.True(t, exists)
		require.Len(t, entry, 1)
		require.Equal(t, storedPubKey, entry[0])
	})

	t.Run("returns empty map if empty account lookup config provided", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{}

		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig, testContractIDL)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, rw, testContractIDL)
		require.NoError(t, err)

		// Filter the lookup table addresses based on which accounts are actually used
		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)
		require.Empty(t, filteredLookupTableMap)
	})

	t.Run("returns empty map if only constant account lookup required", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{
			chainwriter.AccountConstant{
				Name:       "Constant",
				Address:    chainwriter.GetRandomPubKey(t).String(),
				IsSigner:   false,
				IsWritable: false,
			},
		}

		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig, testContractIDL)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, rw, testContractIDL)
		require.NoError(t, err)

		// Filter the lookup table addresses based on which accounts are actually used
		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)
		require.Empty(t, filteredLookupTableMap)
	})
}

func TestChainWriter_SubmitTransaction(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	// mock client
	rw := clientmocks.NewReaderWriter(t)
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	// setup admin key
	adminPk, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	admin := adminPk.PublicKey()

	account1 := chainwriter.GetRandomPubKey(t)
	account2 := chainwriter.GetRandomPubKey(t)

	seed1 := []byte("seed1")
	account3 := mustFindPdaProgramAddress(t, [][]byte{seed1}, solana.SystemProgramID)

	// create lookup table addresses
	seed2 := []byte("seed2")
	programID := solana.MustPublicKeyFromBase58("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE")
	derivedTablePda := mustFindPdaProgramAddress(t, [][]byte{seed2}, programID)
	// mock data account response from program
	derivedLookupTablePubkey := mockDataAccountLookupTable(t, rw, derivedTablePda)
	// mock fetch lookup table addresses call
	derivedLookupKeys := chainwriter.CreateTestPubKeys(t, 1)
	mockFetchLookupTableAddresses(t, rw, derivedLookupTablePubkey, derivedLookupKeys)

	// mock static lookup table call
	staticLookupTablePubkey := chainwriter.GetRandomPubKey(t)
	staticLookupKeys := chainwriter.CreateTestPubKeys(t, 2)
	mockFetchLookupTableAddresses(t, rw, staticLookupTablePubkey, staticLookupKeys)

	cwConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"contract_reader_interface": {
				Methods: map[string]chainwriter.MethodConfig{
					"initializeLookupTable": {
						FromAddress:       admin.String(),
						ChainSpecificName: "initializeLookupTable",
						LookupTables: chainwriter.LookupTables{
							DerivedLookupTables: []chainwriter.DerivedLookupTable{
								{
									Name: "DerivedTable",
									Accounts: chainwriter.PDALookups{
										Name:      "DataAccountPDA",
										PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()},
										Seeds: []chainwriter.Seed{
											// extract seed2 for PDA lookup
											{Dynamic: chainwriter.AccountLookup{Name: "Seed2", Location: "Seed2"}},
										},
										IsSigner:   false,
										IsWritable: false,
										InternalField: chainwriter.InternalField{
											TypeName: "LookupTableDataAccount",
											Location: "LookupTable",
										},
									},
								},
							},
							StaticLookupTables: []solana.PublicKey{staticLookupTablePubkey},
						},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "feepayer",
								Address:    admin.String(),
								IsSigner:   false,
								IsWritable: false,
							},
							chainwriter.AccountConstant{
								Name:       "Constant",
								Address:    account1.String(),
								IsSigner:   false,
								IsWritable: false,
							},
							chainwriter.AccountLookup{
								Name:       "LookupTable",
								Location:   "LookupTable",
								IsSigner:   chainwriter.MetaBool{Value: false},
								IsWritable: chainwriter.MetaBool{Value: false},
							},
							chainwriter.PDALookups{
								Name:      "DataAccountPDA",
								PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: solana.SystemProgramID.String()},
								Seeds: []chainwriter.Seed{
									// extract seed1 for PDA lookup
									{Dynamic: chainwriter.AccountLookup{Name: "Seed1", Location: "Seed1"}},
								},
								IsSigner:   false,
								IsWritable: false,
								// Just get the address of the account, nothing internal.
								InternalField: chainwriter.InternalField{},
							},
							chainwriter.AccountsFromLookupTable{
								LookupTableName: "DerivedTable",
								IncludeIndexes:  []int{0},
							},
							chainwriter.AccountConstant{
								Name:       "systemprogram",
								Address:    solana.SystemProgramID.String(),
								IsSigner:   false,
								IsWritable: false,
							},
						},
						ArgsTransform: "",
					},
				},
				IDL: testContractIDL,
			},
		},
	}

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, cwConfig)
	require.NoError(t, err)

	t.Run("fails with invalid ABI", func(t *testing.T) {
		invalidCWConfig := chainwriter.ChainWriterConfig{
			Programs: map[string]chainwriter.ProgramConfig{
				"invalid_program": {
					Methods: map[string]chainwriter.MethodConfig{
						"invalid": {
							ChainSpecificName: "invalid",
						},
					},
					IDL: "",
				},
			},
		}

		_, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, invalidCWConfig)
		require.Error(t, err)
	})

	t.Run("fails to encode payload if args with missing values provided", func(t *testing.T) {
		txID := uuid.NewString()
		type InvalidArgs struct{}
		args := InvalidArgs{}
		submitErr := cw.SubmitTransaction(ctx, "contract_reader_interface", "initializeLookupTable", args, txID, programID.String(), nil, nil)
		require.Error(t, submitErr)
	})

	t.Run("fails if invalid contract name provided", func(t *testing.T) {
		txID := uuid.NewString()
		args := Arguments{}
		submitErr := cw.SubmitTransaction(ctx, "badContract", "initializeLookupTable", args, txID, programID.String(), nil, nil)
		require.Error(t, submitErr)
	})

	t.Run("fails if invalid method provided", func(t *testing.T) {
		txID := uuid.NewString()

		args := Arguments{}
		submitErr := cw.SubmitTransaction(ctx, "contract_reader_interface", "badMethod", args, txID, programID.String(), nil, nil)
		require.Error(t, submitErr)
	})

	t.Run("submits transaction successfully", func(t *testing.T) {
		recentBlockHash := solana.Hash{}
		rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()
		txID := uuid.NewString()

		txm.On("Enqueue", mock.Anything, admin.String(), mock.MatchedBy(func(tx *solana.Transaction) bool {
			// match transaction fields to ensure it was built as expected
			require.Equal(t, recentBlockHash, tx.Message.RecentBlockhash)
			require.Len(t, tx.Message.Instructions, 1)
			require.Len(t, tx.Message.AccountKeys, 6)                           // fee payer + derived accounts
			require.Equal(t, admin, tx.Message.AccountKeys[0])                  // fee payer
			require.Equal(t, account1, tx.Message.AccountKeys[1])               // account constant
			require.Equal(t, account2, tx.Message.AccountKeys[2])               // account lookup
			require.Equal(t, account3, tx.Message.AccountKeys[3])               // pda lookup
			require.Equal(t, solana.SystemProgramID, tx.Message.AccountKeys[4]) // system program ID
			require.Equal(t, programID, tx.Message.AccountKeys[5])              // instruction program ID
			// instruction program ID
			require.Len(t, tx.Message.AddressTableLookups, 1)                                        // address table look contains entry
			require.Equal(t, derivedLookupTablePubkey, tx.Message.AddressTableLookups[0].AccountKey) // address table
			return true
		}), &txID, mock.Anything).Return(nil).Once()

		args := Arguments{
			LookupTable: account2,
			Seed1:       seed1,
			Seed2:       seed2,
		}

		submitErr := cw.SubmitTransaction(ctx, "contract_reader_interface", "initializeLookupTable", args, txID, programID.String(), nil, nil)
		require.NoError(t, submitErr)
	})
}

func TestChainWriter_CCIPRouter(t *testing.T) {
	t.Parallel()

	// setup admin key
	adminPk, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	admin := adminPk.PublicKey()

	routerAddr := chainwriter.GetRandomPubKey(t)
	destTokenAddr := chainwriter.GetRandomPubKey(t)

	poolKeys := []solana.PublicKey{destTokenAddr}
	poolKeys = append(poolKeys, chainwriter.CreateTestPubKeys(t, 3)...)

	// simplified CCIP Config - does not contain full account list
	ccipCWConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"ccip_router": {
				Methods: map[string]chainwriter.MethodConfig{
					"execute": {
						FromAddress: admin.String(),
						InputModifications: []codec.ModifierConfig{
							&codec.RenameModifierConfig{
								Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
							},
							&codec.RenameModifierConfig{
								Fields: map[string]string{"RawExecutionReport": "Report"},
							},
						},
						ChainSpecificName: "execute",
						ArgsTransform:     "CCIP",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:    "testAcc1",
								Address: chainwriter.GetRandomPubKey(t).String(),
							},
							chainwriter.AccountConstant{
								Name:    "testAcc2",
								Address: chainwriter.GetRandomPubKey(t).String(),
							},
							chainwriter.AccountConstant{
								Name:    "testAcc3",
								Address: chainwriter.GetRandomPubKey(t).String(),
							},
							chainwriter.AccountConstant{
								Name:    "poolAddr1",
								Address: poolKeys[0].String(),
							},
							chainwriter.AccountConstant{
								Name:    "poolAddr2",
								Address: poolKeys[1].String(),
							},
							chainwriter.AccountConstant{
								Name:    "poolAddr3",
								Address: poolKeys[2].String(),
							},
							chainwriter.AccountConstant{
								Name:    "poolAddr4",
								Address: poolKeys[3].String(),
							},
						},
					},
					"commit": {
						FromAddress: admin.String(),
						InputModifications: []codec.ModifierConfig{
							&codec.RenameModifierConfig{
								Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
							},
							&codec.RenameModifierConfig{
								Fields: map[string]string{"RawReport": "Report"},
							},
						},
						ChainSpecificName: "commit",
						ArgsTransform:     "",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:    "testAcc1",
								Address: chainwriter.GetRandomPubKey(t).String(),
							},
							chainwriter.AccountConstant{
								Name:    "testAcc2",
								Address: chainwriter.GetRandomPubKey(t).String(),
							},
							chainwriter.AccountConstant{
								Name:    "testAcc3",
								Address: chainwriter.GetRandomPubKey(t).String(),
							},
						},
					},
				},
				IDL: ccipRouterIDL,
			},
		},
	}

	ctx := tests.Context(t)
	// mock client
	rw := clientmocks.NewReaderWriter(t)
	// mock estimator
	ge := feemocks.NewEstimator(t)

	t.Run("CCIP execute is encoded successfully and ArgsTransform is applied correctly.", func(t *testing.T) {
		// mock txm
		txm := txmMocks.NewTxManager(t)
		// initialize chain writer
		cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, ccipCWConfig)
		require.NoError(t, err)

		recentBlockHash := solana.Hash{}
		rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()

		pda, _, err := solana.FindProgramAddress([][]byte{[]byte("token_admin_registry"), destTokenAddr.Bytes()}, routerAddr)
		require.NoError(t, err)

		lookupTable := mockTokenAdminRegistryLookupTable(t, rw, pda)

		mockFetchLookupTableAddresses(t, rw, lookupTable, poolKeys)

		txID := uuid.NewString()
		txm.On("Enqueue", mock.Anything, admin.String(), mock.MatchedBy(func(tx *solana.Transaction) bool {
			txData := tx.Message.Instructions[0].Data
			payload := txData[8:]
			var decoded ccip_router.Execute
			dec := ag_binary.NewBorshDecoder(payload)
			err = dec.Decode(&decoded)
			require.NoError(t, err)

			tokenIndexes := *decoded.TokenIndexes

			require.Len(t, tokenIndexes, 1)
			require.Equal(t, uint8(3), tokenIndexes[0])
			return true
		}), &txID, mock.Anything).Return(nil).Once()

		// stripped back report just for purposes of example
		abstractReport := ccipocr3.ExecutePluginReportSingleChain{
			Messages: []ccipocr3.Message{
				{
					TokenAmounts: []ccipocr3.RampTokenAmount{
						{
							DestTokenAddress: destTokenAddr.Bytes(),
						},
					},
				},
			},
		}

		// Marshal the abstract report to json just for testing purposes.
		encodedReport, err := json.Marshal(abstractReport)
		require.NoError(t, err)

		args := chainwriter.ReportPreTransform{
			ReportContext: [2][32]byte{{0x01}, {0x02}},
			Report:        encodedReport,
			Info: ccipocr3.ExecuteReportInfo{
				MerkleRoots:     []ccipocr3.MerkleRootChain{},
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{abstractReport},
			},
		}

		submitErr := cw.SubmitTransaction(ctx, "ccip_router", "execute", args, txID, routerAddr.String(), nil, nil)
		require.NoError(t, submitErr)
	})

	t.Run("CCIP commit is encoded successfully", func(t *testing.T) {
		// mock txm
		txm := txmMocks.NewTxManager(t)
		// initialize chain writer
		cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, ccipCWConfig)
		require.NoError(t, err)

		recentBlockHash := solana.Hash{}
		rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()

		type CommitArgs struct {
			ReportContext [2][32]byte
			Report        []byte
			Rs            [][32]byte
			Ss            [][32]byte
			RawVs         [32]byte
			Info          ccipocr3.CommitReportInfo
		}

		txID := uuid.NewString()

		// TODO: Replace with actual type from ccipocr3
		args := CommitArgs{
			ReportContext: [2][32]byte{{0x01}, {0x02}},
			Report:        []byte{0x01, 0x02},
			Rs:            [][32]byte{{0x01, 0x02}},
			Ss:            [][32]byte{{0x01, 0x02}},
			RawVs:         [32]byte{0x01, 0x02},
			Info: ccipocr3.CommitReportInfo{
				RemoteF:     1,
				MerkleRoots: []ccipocr3.MerkleRootChain{},
			},
		}

		txm.On("Enqueue", mock.Anything, admin.String(), mock.MatchedBy(func(tx *solana.Transaction) bool {
			txData := tx.Message.Instructions[0].Data
			payload := txData[8:]
			var decoded ccip_router.Commit
			dec := ag_binary.NewBorshDecoder(payload)
			err := dec.Decode(&decoded)
			require.NoError(t, err)
			return true
		}), &txID, mock.Anything).Return(nil).Once()

		submitErr := cw.SubmitTransaction(ctx, "ccip_router", "commit", args, txID, routerAddr.String(), nil, nil)
		require.NoError(t, submitErr)
	})
}

func TestChainWriter_GetTransactionStatus(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	rw := clientmocks.NewReaderWriter(t)
	ge := feemocks.NewEstimator(t)

	// mock txm
	txm := txmMocks.NewTxManager(t)

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	t.Run("returns unknown with error if ID not found", func(t *testing.T) {
		txID := uuid.NewString()
		txm.On("GetTransactionStatus", mock.Anything, txID).Return(types.Unknown, errors.New("tx not found")).Once()
		status, err := cw.GetTransactionStatus(ctx, txID)
		require.Error(t, err)
		require.Equal(t, types.Unknown, status)
	})

	t.Run("returns pending when transaction is pending", func(t *testing.T) {
		txID := uuid.NewString()
		txm.On("GetTransactionStatus", mock.Anything, txID).Return(types.Pending, nil).Once()
		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Pending, status)
	})

	t.Run("returns unconfirmed when transaction is unconfirmed", func(t *testing.T) {
		txID := uuid.NewString()
		txm.On("GetTransactionStatus", mock.Anything, txID).Return(types.Unconfirmed, nil).Once()
		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Unconfirmed, status)
	})

	t.Run("returns finalized when transaction is finalized", func(t *testing.T) {
		txID := uuid.NewString()
		txm.On("GetTransactionStatus", mock.Anything, txID).Return(types.Finalized, nil).Once()
		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)
	})

	t.Run("returns failed when transaction error classfied as failed", func(t *testing.T) {
		txID := uuid.NewString()
		txm.On("GetTransactionStatus", mock.Anything, txID).Return(types.Failed, nil).Once()
		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Failed, status)
	})

	t.Run("returns fatal when transaction error classfied as fatal", func(t *testing.T) {
		txID := uuid.NewString()
		txm.On("GetTransactionStatus", mock.Anything, txID).Return(types.Fatal, nil).Once()
		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Fatal, status)
	})
}

func TestChainWriter_GetFeeComponents(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	rw := clientmocks.NewReaderWriter(t)
	ge := feemocks.NewEstimator(t)
	ge.On("BaseComputeUnitPrice").Return(uint64(100))

	// mock txm
	txm := txmMocks.NewTxManager(t)

	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	t.Run("returns valid compute unit price", func(t *testing.T) {
		feeComponents, err := cw.GetFeeComponents(ctx)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(100), feeComponents.ExecutionFee)
		require.Nil(t, feeComponents.DataAvailabilityFee) // always nil for Solana
	})

	t.Run("fails if gas estimator not set", func(t *testing.T) {
		cwNoEstimator, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), rw, txm, nil, chainwriter.ChainWriterConfig{})
		require.NoError(t, err)
		_, err = cwNoEstimator.GetFeeComponents(ctx)
		require.Error(t, err)
	})
}

func mustBorshEncodeStruct(t *testing.T, data interface{}) []byte {
	buf := new(bytes.Buffer)
	err := ag_binary.NewBorshEncoder(buf).Encode(data)
	require.NoError(t, err)
	return buf.Bytes()
}

func mustFindPdaProgramAddress(t *testing.T, seeds [][]byte, programID solana.PublicKey) solana.PublicKey {
	pda, _, err := solana.FindProgramAddress(seeds, programID)
	require.NoError(t, err)
	return pda
}

func mockDataAccountLookupTable(t *testing.T, rw *clientmocks.ReaderWriter, pda solana.PublicKey) solana.PublicKey {
	lookupTablePubkey := chainwriter.GetRandomPubKey(t)
	dataAccount := chainwriter.DataAccount{
		Version:              1,
		Administrator:        chainwriter.GetRandomPubKey(t),
		PendingAdministrator: chainwriter.GetRandomPubKey(t),
		LookupTable:          lookupTablePubkey,
	}
	dataAccountBytes := mustBorshEncodeStruct(t, dataAccount)
	// codec will expect discriminator
	dataAccountBytes = append([]byte{220, 119, 44, 40, 237, 41, 223, 7}, dataAccountBytes...)
	rw.On("GetAccountInfoWithOpts", mock.Anything, pda, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(dataAccountBytes)},
	}, nil)
	return lookupTablePubkey
}

func mockTokenAdminRegistryLookupTable(t *testing.T, rw *clientmocks.ReaderWriter, pda solana.PublicKey) solana.PublicKey {
	lookupTablePubkey := chainwriter.GetRandomPubKey(t)
	tokenAdminRegistry := ccip_router.TokenAdminRegistry{
		Version:              1,
		Administrator:        chainwriter.GetRandomPubKey(t),
		PendingAdministrator: chainwriter.GetRandomPubKey(t),
		LookupTable:          lookupTablePubkey,
		WritableIndexes:      [2]ag_binary.Uint128{},
	}
	registryBytes := mustBorshEncodeStruct(t, tokenAdminRegistry)
	rw.On("GetAccountInfoWithOpts", mock.Anything, pda, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(registryBytes)},
	}, nil)
	return lookupTablePubkey
}

func mockFetchLookupTableAddresses(t *testing.T, rw *clientmocks.ReaderWriter, lookupTablePubkey solana.PublicKey, storedPubkeys []solana.PublicKey) {
	var lookupTablePubkeySlice solana.PublicKeySlice
	lookupTablePubkeySlice.Append(storedPubkeys...)
	lookupTableState := addresslookuptable.AddressLookupTableState{
		Addresses: lookupTablePubkeySlice,
	}
	lookupTableStateBytes := mustBorshEncodeStruct(t, lookupTableState)
	rw.On("GetAccountInfoWithOpts", mock.Anything, lookupTablePubkey, mock.Anything).Return(&rpc.GetAccountInfoResult{
		RPCContext: rpc.RPCContext{},
		Value:      &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(lookupTableStateBytes)},
	}, nil)
}
