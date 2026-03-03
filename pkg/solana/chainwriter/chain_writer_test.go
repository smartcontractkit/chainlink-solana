package chainwriter_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
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
	ccipconsts "github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_1/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	codecv1TestUtils "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1/testutils"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
	feemocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/fees/mocks"
	txmMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

type Arguments struct {
	LookupTable solana.PublicKey
	Seed1       []byte
	Seed2       []byte
}

type BufferArgs struct {
	Report []byte
	Fail   bool
}

var (
	ccipOfframpIDL        = ccipsolana.FetchCCIPOfframpIDL()
	testContractIDL       = chainwriter.FetchTestContractIDL()
	testBufferContractIDL = chainwriter.FetchTestBufferContractIDL()
)

func TestChainWriter_GetAddresses(t *testing.T) {
	ctx := t.Context()

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, chainwriter.ChainWriterConfig{})
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
	programID := GetRandomPubKey(t)
	seed2 := []byte("seed2")
	pda2 := mustFindPdaProgramAddress(t, [][]byte{seed2}, programID)
	// mock data account response from program
	lookupTablePubkey := mockDataAccountLookupTable(t, rw, pda2)
	// mock fetch lookup table addresses call
	storedPubKeys := CreateTestPubKeys(t, 3)
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
				Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
					Name:      "DataAccountPDA",
					PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()}},
					Seeds: []chainwriter.Seed{
						// extract seed2 for PDA lookup
						{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "Seed2", Location: "Seed2"}}},
					},
					IsSigner:   derivedTablePdaLookupMeta.IsSigner,
					IsWritable: derivedTablePdaLookupMeta.IsWritable,
					InternalField: chainwriter.InternalField{
						TypeName: "LookupTableDataAccount",
						Location: "LookupTable",
						IDL:      testContractIDL,
					},
				}},
			},
		},
		StaticLookupTables: nil,
	}

	t.Run("resolve addresses from different types of lookups", func(t *testing.T) {
		constantAccountMeta.PublicKey = GetRandomPubKey(t)
		accountLookupMeta.PublicKey = GetRandomPubKey(t)
		// correlates to DerivedTable index in account lookup config
		derivedTablePdaLookupMeta.PublicKey = storedPubKeys[0]

		args := Arguments{
			LookupTable: accountLookupMeta.PublicKey,
			Seed1:       seed1,
			Seed2:       seed2,
		}

		accountLookupConfig := []chainwriter.Lookup{
			{AccountConstant: &chainwriter.AccountConstant{
				Name:       "Constant",
				Address:    constantAccountMeta.PublicKey.String(),
				IsSigner:   constantAccountMeta.IsSigner,
				IsWritable: constantAccountMeta.IsWritable,
			}},
			{AccountLookup: &chainwriter.AccountLookup{
				Name:       "LookupTable",
				Location:   "LookupTable",
				IsSigner:   chainwriter.MetaBool{Value: accountLookupMeta.IsSigner},
				IsWritable: chainwriter.MetaBool{Value: accountLookupMeta.IsWritable},
			}},
			{PDALookups: &chainwriter.PDALookups{
				Name:      "DataAccountPDA",
				PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: solana.SystemProgramID.String()}},
				Seeds: []chainwriter.Seed{
					// extract seed1 for PDA lookup
					{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "Seed1", Location: "Seed1"}}},
				},
				IsSigner:   pdaLookupMeta.IsSigner,
				IsWritable: pdaLookupMeta.IsWritable,
				// Just get the address of the account, nothing internal.
				InternalField: chainwriter.InternalField{},
			}},
			{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0},
			}},
		}

		// Fetch derived table map
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
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
			{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0, 2},
			}},
		}

		// Fetch derived table map
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
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
			{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
			}},
		}

		// Fetch derived table map
		derivedTableMap, _, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
		require.NoError(t, err)

		require.Len(t, accounts, 3)
		for i, storedPubkey := range storedPubKeys {
			require.Equal(t, storedPubkey, accounts[i].PublicKey)
		}
	})

	t.Run("optional lookups", func(t *testing.T) {
		const invalidLocation = "Invalid.Path"

		t.Run("AccountLookup error is skipped when Lookup is optional", func(t *testing.T) {
			accountLookupConfig := []chainwriter.Lookup{
				{
					AccountLookup: &chainwriter.AccountLookup{
						Name:       "OptionalAccountLookup",
						Location:   invalidLocation,
						IsSigner:   chainwriter.MetaBool{Value: false},
						IsWritable: chainwriter.MetaBool{Value: false},
					},
					Optional: true,
				},
			}

			args := Arguments{}

			accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, nil, mc)
			require.NoError(t, err)
			require.Empty(t, accounts)
		})

		t.Run("AccountLookup error is returned when Lookup is required", func(t *testing.T) {
			accountLookupConfig := []chainwriter.Lookup{
				{AccountLookup: &chainwriter.AccountLookup{
					Name:       "NonOptionalAccountLookup",
					Location:   invalidLocation,
					IsSigner:   chainwriter.MetaBool{Value: false},
					IsWritable: chainwriter.MetaBool{Value: false},
				}},
			}

			args := Arguments{}
			accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, nil, mc)
			require.Error(t, err)
			require.Nil(t, accounts)
		})

		t.Run("PDALookups error is skipped when Lookup is optional", func(t *testing.T) {
			accountLookupConfig := []chainwriter.Lookup{
				{
					PDALookups: &chainwriter.PDALookups{
						Name:      "OptionalPDA",
						PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: solana.SystemProgramID.String()}},
						Seeds: []chainwriter.Seed{
							{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Location: invalidLocation}}},
						},
					},
					Optional: true,
				},
			}

			args := Arguments{}
			accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, nil, mc)
			require.NoError(t, err)
			require.Empty(t, accounts)
		})

		t.Run("PDALookups error is returned when Lookup is required", func(t *testing.T) {
			accountLookupConfig := []chainwriter.Lookup{
				{PDALookups: &chainwriter.PDALookups{
					Name:      "NonOptionalPDA",
					PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "ProgramID", Address: solana.SystemProgramID.String()}},
					Seeds: []chainwriter.Seed{
						{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Location: invalidLocation}}},
					},
				}},
			}

			args := Arguments{}
			accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, nil, mc)
			require.Error(t, err)
			require.Nil(t, accounts)
		})

		t.Run("DerivedLookupTable error is skipped when Lookup is optional", func(t *testing.T) {
			lookupTables := chainwriter.LookupTables{
				DerivedLookupTables: []chainwriter.DerivedLookupTable{
					{
						Name:     "OptionalDerivedTable",
						Optional: true,
						Accounts: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
							Location: invalidLocation,
						}},
					},
				},
			}

			args := Arguments{}
			derivedMap, staticMap, err := cw.ResolveLookupTables(ctx, args, lookupTables)
			require.NoError(t, err)
			require.Empty(t, derivedMap)
			require.Empty(t, staticMap)
		})

		t.Run("DerivedLookupTable error is returned when Lookup is required", func(t *testing.T) {
			lookupTables := chainwriter.LookupTables{
				DerivedLookupTables: []chainwriter.DerivedLookupTable{
					{
						Name: "NonOptionalDerivedTable",
						Accounts: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{
							Location: invalidLocation,
						}},
						Optional: false,
					},
				},
			}

			args := Arguments{}
			_, _, err := cw.ResolveLookupTables(ctx, args, lookupTables)
			require.Error(t, err)
		})

		t.Run("AccountsFromLookupTable error is skipped when Lookup is optional", func(t *testing.T) {
			accountLookupConfig := []chainwriter.Lookup{
				{
					AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
						LookupTableName: "NonExistent",
					},
					Optional: true,
				},
			}

			args := Arguments{}

			accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, nil, mc)
			require.NoError(t, err)
			require.Empty(t, accounts)
		})

		t.Run("AccountsFromLookupTable error is returned when Lookup is required", func(t *testing.T) {
			accountLookupConfig := []chainwriter.Lookup{
				{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
					LookupTableName: "NonExistent",
				}},
			}

			args := Arguments{}
			_, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, nil, mc)
			require.Error(t, err)
		})
	})
}

func TestChainWriter_FilterLookupTableAddresses(t *testing.T) {
	ctx := t.Context()

	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	programID := GetRandomPubKey(t)
	seed1 := []byte("seed1")
	pda1 := mustFindPdaProgramAddress(t, [][]byte{seed1}, programID)
	// mock data account response from program
	lookupTablePubkey := mockDataAccountLookupTable(t, rw, pda1)
	// mock fetch lookup table addresses call
	storedPubKey := GetRandomPubKey(t)
	unusedKeys := CreateTestPubKeys(t, 2)
	mockFetchLookupTableAddresses(t, rw, lookupTablePubkey, append([]solana.PublicKey{storedPubKey}, unusedKeys...))

	unusedProgramID := GetRandomPubKey(t)
	seed2 := []byte("seed2")
	unusedPda := mustFindPdaProgramAddress(t, [][]byte{seed2}, unusedProgramID)
	// mock data account response from program
	unusedLookupTable := mockDataAccountLookupTable(t, rw, unusedPda)
	// mock fetch lookup table addresses call
	unusedKeys = CreateTestPubKeys(t, 2)
	mockFetchLookupTableAddresses(t, rw, unusedLookupTable, unusedKeys)

	// mock static lookup table calls
	staticLookupTablePubkey1 := GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, staticLookupTablePubkey1, CreateTestPubKeys(t, 2))
	staticLookupTablePubkey2 := GetRandomPubKey(t)
	mockFetchLookupTableAddresses(t, rw, staticLookupTablePubkey2, CreateTestPubKeys(t, 2))

	lookupTableConfig := chainwriter.LookupTables{
		DerivedLookupTables: []chainwriter.DerivedLookupTable{
			{
				Name: "DerivedTable",
				Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
					Name:      "DataAccountPDA",
					PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()}},
					Seeds: []chainwriter.Seed{
						// extract seed1 for PDA lookup
						{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "Seed1", Location: "Seed1"}}},
					},
					IsSigner:   true,
					IsWritable: true,
					InternalField: chainwriter.InternalField{
						TypeName: "LookupTableDataAccount",
						Location: "LookupTable",
						IDL:      testContractIDL,
					},
				}},
			},
			{
				Name: "MiscDerivedTable",
				Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
					Name:      "MiscPDA",
					PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "UnusedAccount", Address: unusedProgramID.String()}},
					Seeds: []chainwriter.Seed{
						// extract seed2 for PDA lookup
						{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "Seed2", Location: "Seed2"}}},
					},
					IsSigner:   true,
					IsWritable: true,
					InternalField: chainwriter.InternalField{
						TypeName: "LookupTableDataAccount",
						Location: "LookupTable",
						IDL:      testContractIDL,
					},
				}},
			},
		},
		StaticLookupTables: []solana.PublicKey{staticLookupTablePubkey1, staticLookupTablePubkey2},
	}

	args := Arguments{
		Seed1: seed1,
		Seed2: seed2,
	}

	t.Run("returns filtered map with only relevant lookup tables required by account lookup config", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{
			{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0},
			}},
		}

		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
		require.NoError(t, err)

		// Filter the lookup table addresses based on which accounts are actually used
		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)

		// Filter map should only contain the address for the DerivedTable lookup defined in the account lookup config
		require.Len(t, filteredLookupTableMap, len(accounts))
		entry, exists := filteredLookupTableMap[lookupTablePubkey]
		require.True(t, exists)
		require.Len(t, entry, 3)
		require.Equal(t, storedPubKey, entry[0])
	})

	t.Run("returns filtered map and ignores nil account meta and nil entry in lookup table", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{
			{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
				LookupTableName: "DerivedTable",
				IncludeIndexes:  []int{0},
			}},
		}
		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		lookupAccounts := derivedTableMap["DerivedTable"][lookupTablePubkey.String()]
		// add nil account meta
		lookupAccounts = append(lookupAccounts, nil)
		derivedTableMap["DerivedTable"][lookupTablePubkey.String()] = lookupAccounts

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
		require.NoError(t, err)

		// add nil account meta
		accounts = append(accounts, nil)

		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)
		// Filter map should only contain the address for the DerivedTable lookup defined in the account lookup config
		require.Len(t, filteredLookupTableMap, 1)
		entry, exists := filteredLookupTableMap[lookupTablePubkey]
		require.True(t, exists)
		require.Len(t, entry, 3)
		require.Equal(t, storedPubKey, entry[0])
	})

	t.Run("returns empty map if empty account lookup config provided", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{}

		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
		require.NoError(t, err)

		// Filter the lookup table addresses based on which accounts are actually used
		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)
		require.Empty(t, filteredLookupTableMap)
	})

	t.Run("returns empty map if only constant account lookup required", func(t *testing.T) {
		accountLookupConfig := []chainwriter.Lookup{
			{AccountConstant: &chainwriter.AccountConstant{
				Name:       "Constant",
				Address:    GetRandomPubKey(t).String(),
				IsSigner:   false,
				IsWritable: false,
			}},
		}

		// Fetch derived table map
		derivedTableMap, staticTableMap, err := cw.ResolveLookupTables(ctx, args, lookupTableConfig)
		require.NoError(t, err)

		// Resolve account metas
		accounts, err := chainwriter.GetAddresses(ctx, args, accountLookupConfig, derivedTableMap, mc)
		require.NoError(t, err)

		// Filter the lookup table addresses based on which accounts are actually used
		filteredLookupTableMap := cw.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)
		require.Empty(t, filteredLookupTableMap)
	})
}

func TestChainWriter_SubmitTransaction(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	// setup admin key
	adminPk, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	admin := adminPk.PublicKey()

	account1 := GetRandomPubKey(t)
	account2 := GetRandomPubKey(t)

	seed1 := []byte("seed1")
	account3 := mustFindPdaProgramAddress(t, [][]byte{seed1}, solana.SystemProgramID)

	// create lookup table addresses
	seed2 := []byte("seed2")
	programID := solana.MustPublicKeyFromBase58("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE")
	bufferProgramID := solana.MustPublicKeyFromBase58("85bivLENWAX36kyWC9zemZu9H3D88J79wXdHgR6ZmZHX")
	derivedTablePda := mustFindPdaProgramAddress(t, [][]byte{seed2}, programID)
	// mock data account response from program
	derivedLookupTablePubkey := mockDataAccountLookupTable(t, rw, derivedTablePda)
	// mock fetch lookup table addresses call
	derivedLookupKeys := CreateTestPubKeys(t, 1)
	mockFetchLookupTableAddresses(t, rw, derivedLookupTablePubkey, derivedLookupKeys)

	// mock static lookup table call
	staticLookupTablePubkey := GetRandomPubKey(t)
	staticLookupKeys := CreateTestPubKeys(t, 2)
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
									Accounts: chainwriter.Lookup{PDALookups: &chainwriter.PDALookups{
										Name:      "DataAccountPDA",
										PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: programID.String()}},
										Seeds: []chainwriter.Seed{
											// extract seed2 for PDA lookup
											{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "Seed2", Location: "Seed2"}}},
										},
										IsSigner:   false,
										IsWritable: false,
										InternalField: chainwriter.InternalField{
											TypeName: "LookupTableDataAccount",
											Location: "LookupTable",
											IDL:      testContractIDL,
										},
									}},
								},
							},
							StaticLookupTables: []solana.PublicKey{staticLookupTablePubkey},
						},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "feepayer",
								Address:    admin.String(),
								IsSigner:   false,
								IsWritable: false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "Constant",
								Address:    account1.String(),
								IsSigner:   false,
								IsWritable: false,
							}},
							{AccountLookup: &chainwriter.AccountLookup{
								Name:       "LookupTable",
								Location:   "LookupTable",
								IsSigner:   chainwriter.MetaBool{Value: false},
								IsWritable: chainwriter.MetaBool{Value: false},
							}},
							{PDALookups: &chainwriter.PDALookups{
								Name:      "DataAccountPDA",
								PublicKey: chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{Name: "WriteTest", Address: solana.SystemProgramID.String()}},
								Seeds: []chainwriter.Seed{
									// extract seed1 for PDA lookup
									{Dynamic: chainwriter.Lookup{AccountLookup: &chainwriter.AccountLookup{Name: "Seed1", Location: "Seed1"}}},
								},
								IsSigner:   false,
								IsWritable: false,
								// Just get the address of the account, nothing internal.
								InternalField: chainwriter.InternalField{},
							}},
							{AccountsFromLookupTable: &chainwriter.AccountsFromLookupTable{
								LookupTableName: "DerivedTable",
								IncludeIndexes:  []int{0},
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "systemprogram",
								Address:    solana.SystemProgramID.String(),
								IsSigner:   false,
								IsWritable: false,
							}},
						},
						ArgsTransform: "",
					},
				},
				IDL: testContractIDL,
			},
			"buffer_payload": {
				Methods: map[string]chainwriter.MethodConfig{
					"execute": {
						FromAddress:         admin.String(),
						ChainSpecificName:   "execute",
						BufferPayloadMethod: "CCIPExecutionReportBuffer",
						InputModifications: codec.ModifiersConfig{
							&codec.HardCodeModifierConfig{OnChainValues: map[string]any{"Fail": false}},
						},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "feepayer",
								Address:    admin.String(),
								IsSigner:   false,
								IsWritable: false,
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:       "system",
								Address:    solana.SystemProgramID.String(),
								IsSigner:   false,
								IsWritable: false,
							}},
						},
					},
				},
				IDL: testBufferContractIDL,
			},
		},
	}

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, cwConfig)
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

		_, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, invalidCWConfig)
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

	t.Run("invalid buffer methods", func(t *testing.T) {
		recentBlockHash := solana.Hash{}

		customConfig := chainwriter.ChainWriterConfig{
			Programs: map[string]chainwriter.ProgramConfig{
				"buffer_payload": {
					Methods: map[string]chainwriter.MethodConfig{
						"execute": {
							FromAddress:       admin.String(),
							ChainSpecificName: "execute",
							InputModifications: codec.ModifiersConfig{
								&codec.HardCodeModifierConfig{OnChainValues: map[string]any{"Fail": false}},
							},
							Accounts: []chainwriter.Lookup{},
						},
					},
					IDL: testBufferContractIDL,
				},
			},
		}

		args := BufferArgs{
			Report: make([]byte, 2000),
			Fail:   false,
		}

		t.Run("fails to submit transaction if size too large without buffer method set", func(t *testing.T) {
			rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()
			txID := uuid.NewString()

			// initialize chain writer
			customCW, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, customConfig)
			require.NoError(t, err)

			submitErr := customCW.SubmitTransaction(ctx, "buffer_payload", "execute", args, txID, programID.String(), nil, nil)
			require.Error(t, submitErr)
		})

		t.Run("fails to submit transaction if unknown buffer payload method configured", func(t *testing.T) {
			rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()
			txID := uuid.NewString()

			methodConfig := customConfig.Programs["buffer_payload"].Methods["execute"]
			methodConfig.BufferPayloadMethod = "BadBufferPayloadMethod"
			customConfig.Programs["buffer_payload"].Methods["execute"] = methodConfig
			// initialize chain writer
			customCW, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, customConfig)
			require.NoError(t, err)

			submitErr := customCW.SubmitTransaction(ctx, "buffer_payload", "execute", args, txID, programID.String(), nil, nil)
			require.Error(t, submitErr)
		})
	})

	t.Run("buffer enabled method", func(t *testing.T) {
		recentBlockHash := solana.Hash{}
		// mock txm
		bufferTXM := txmMocks.NewTxManager(t)
		// initialize chain writer
		bufferCW, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, bufferTXM, ge, cwConfig)
		require.NoError(t, err)

		t.Run("submits as single transaction if tx small enough", func(t *testing.T) {
			args := BufferArgs{
				Report: make([]byte, 963),
				Fail:   false,
			}
			ix := buildExecuteIxExact(bufferProgramID, admin, args.Report, args.Fail)

			tx, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{}, solana.TransactionPayer(admin))
			require.NoError(t, err)
			txSize, err := chainwriter.CalculateTxSize(tx)
			require.NoError(t, err)
			require.Equal(t, chainwriter.MaxSolanaTxSize, txSize)

			rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()
			txID := uuid.NewString()

			bufferTXM.On("Enqueue", mock.Anything, admin.String(), mock.Anything, &txID, mock.Anything).Return(nil).Once()

			submitErr := bufferCW.SubmitTransaction(ctx, "buffer_payload", "execute", args, txID, bufferProgramID.String(), nil, nil)
			require.NoError(t, submitErr)
		})

		t.Run("submits buffer transactions, main transaction, and conditional close buffer transaction if tx too large", func(t *testing.T) {
			args := BufferArgs{
				Report: make([]byte, 964),
				Fail:   false,
			}

			ix := buildExecuteIxExact(bufferProgramID, admin, args.Report, args.Fail)

			tx, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{}, solana.TransactionPayer(admin))
			require.NoError(t, err)
			txSize, err := chainwriter.CalculateTxSize(tx)
			require.NoError(t, err)
			require.Equal(t, chainwriter.MaxSolanaTxSize+1, txSize)

			rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Twice()
			txID := uuid.NewString()

			// Buffer tx with estimate limit opt
			bufferTXM.On("Enqueue", mock.Anything, admin.String(), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Twice()
			// Buffer tx with dependency IDs opt
			bufferTXM.On("Enqueue", mock.Anything, admin.String(), mock.Anything, &txID, mock.Anything, mock.Anything).Return(nil).Once()
			// Close buffer with estimate limit, dependency ID, and ignore dependency error opts
			bufferTXM.On("Enqueue", mock.Anything, admin.String(), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

			submitErr := bufferCW.SubmitTransaction(ctx, "buffer_payload", "execute", args, txID, bufferProgramID.String(), nil, nil)
			require.NoError(t, submitErr)
		})

		t.Run("fails if transaction is still too large after buffering", func(t *testing.T) {
			rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Twice()
			txID := uuid.NewString()

			customConfig := chainwriter.ChainWriterConfig{
				Programs: map[string]chainwriter.ProgramConfig{
					"buffer_payload": {
						Methods: map[string]chainwriter.MethodConfig{
							"execute": {
								FromAddress:         admin.String(),
								ChainSpecificName:   "execute",
								BufferPayloadMethod: "CCIPExecutionReportBuffer",
								InputModifications: codec.ModifiersConfig{
									&codec.HardCodeModifierConfig{OnChainValues: map[string]any{"Fail": false}},
								},
								Accounts: []chainwriter.Lookup{},
							},
						},
						IDL: testBufferContractIDL,
					},
				},
			}

			methodConfig := customConfig.Programs["buffer_payload"].Methods["execute"]
			for i := range 40 {
				methodConfig.Accounts = append(methodConfig.Accounts, chainwriter.Lookup{AccountConstant: &chainwriter.AccountConstant{
					Name:       fmt.Sprintf("randomAccount%d", i),
					Address:    GetRandomPubKey(t).String(),
					IsSigner:   false,
					IsWritable: false,
				}})
			}
			customConfig.Programs["buffer_payload"].Methods["execute"] = methodConfig

			args := BufferArgs{
				Report: make([]byte, 2000),
				Fail:   false,
			}

			// initialize chain writer
			customCW, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, customConfig)
			require.NoError(t, err)

			submitErr := customCW.SubmitTransaction(ctx, "buffer_payload", "execute", args, txID, bufferProgramID.String(), nil, nil)
			require.Error(t, submitErr)
		})
	})
}

func buildExecuteIxExact(bufferProgramID solana.PublicKey, admin solana.PublicKey, report []byte, fail bool) solana.Instruction {
	buf := new(bytes.Buffer)
	enc := ag_binary.NewBorshEncoder(buf)

	execute := ag_binary.TypeID([8]byte{130, 221, 242, 154, 13, 193, 189, 29})
	// 1) write variant index exactly like generated BaseVariant would
	_ = enc.Encode(execute) // uint8
	// 2) encode params exactly like the generated MarshalWithEncoder
	//    generated does: encoder.Encode(obj.Report); encoder.Encode(obj.Fail)
	_ = enc.Encode(&report)
	_ = enc.Encode(&fail)

	data := buf.Bytes()

	metas := solana.AccountMetaSlice{
		solana.Meta(admin).WRITE().SIGNER(),
		solana.Meta(solana.SystemProgramID),
	}

	return solana.NewInstruction(bufferProgramID, metas, data)
}

func TestChainWriter_CCIPOfframp(t *testing.T) {
	t.Parallel()

	// setup admin key
	adminPk, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	admin := adminPk.PublicKey()

	offrampAddr := GetRandomPubKey(t)
	destTokenAddr := GetRandomPubKey(t)
	sourcePoolAddr := GetRandomPubKey(t)

	poolKeys := []solana.PublicKey{destTokenAddr}
	poolKeys = append(poolKeys, CreateTestPubKeys(t, 6)...)

	staticCUOverhead := uint32(150_000)

	// simplified CCIP Config - does not contain full account list
	ccipCWConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			ccipconsts.ContractNameOffRamp: {
				Methods: map[string]chainwriter.MethodConfig{
					ccipconsts.MethodExecute: {
						FromAddress: admin.String(),
						InputModifications: []codec.ModifierConfig{
							&codec.RenameModifierConfig{
								Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
							},
							&codec.RenameModifierConfig{
								Fields: map[string]string{"RawExecutionReport": "Report"},
							},
						},
						ChainSpecificName:        "execute",
						ArgsTransform:            "CCIPExecuteV2",
						ComputeUnitLimitOverhead: staticCUOverhead,
					},
					ccipconsts.MethodCommit: {
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
						ArgsTransform:     "CCIPCommit",
						LookupTables:      chainwriter.LookupTables{},
						Accounts: []chainwriter.Lookup{
							{AccountConstant: &chainwriter.AccountConstant{
								Name:    "testAcc1",
								Address: GetRandomPubKey(t).String(),
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:    "testAcc2",
								Address: GetRandomPubKey(t).String(),
							}},
							{AccountConstant: &chainwriter.AccountConstant{
								Name:    "testAcc3",
								Address: GetRandomPubKey(t).String(),
							}},
						},
					},
				},
				IDL: ccipOfframpIDL,
			},
		},
	}

	ctx := t.Context()
	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	// mock estimator
	ge := feemocks.NewEstimator(t)

	t.Run("CCIP execute is encoded successfully and ArgsTransform is applied correctly.", func(t *testing.T) {
		// mock txm
		txm := txmMocks.NewTxManager(t)
		// initialize chain writer
		cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, ccipCWConfig)
		require.NoError(t, err)

		recentBlockHash := solana.Hash{}
		rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()

		feeQuoterAddr := GetRandomPubKey(t)

		logicReceiver := GetRandomPubKey(t)
		userMessagingAccount := GetRandomPubKey(t)
		tokenReceiver := GetRandomPubKey(t)
		sourceChainSel := ccipocr3.ChainSelector(1)
		sourceChainSelBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(sourceChainSelBytes, uint64(sourceChainSel))

		poolProgram := poolKeys[2]
		tokenProgram := poolKeys[6]

		offrampPoolsSigner1, _, err := state.FindExternalTokenPoolsSignerPDA(poolProgram, offrampAddr)
		require.NoError(t, err)
		userTokenAccount1, _, err := solana.FindProgramAddress([][]byte{tokenReceiver.Bytes(), tokenProgram.Bytes(), destTokenAddr.Bytes()}, solana.SPLAssociatedTokenAccountProgramID)
		require.NoError(t, err)
		perChainTokenConfig1, _, err := state.FindFqPerChainPerTokenConfigPDA(uint64(sourceChainSel), destTokenAddr, feeQuoterAddr)
		require.NoError(t, err)
		poolChainConfig1, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelBytes, destTokenAddr.Bytes()}, poolProgram)
		require.NoError(t, err)
		ttAccount := tokenTransferAccounts{
			offrampPoolSigner:   offrampPoolsSigner1,
			userTokenAccount:    userTokenAccount1,
			perChainTokenConfig: perChainTokenConfig1,
			poolChainConfig:     poolChainConfig1,
			poolKeys:            poolKeys,
			mint:                destTokenAddr,
		}

		lookupTablePubkey := GetRandomPubKey(t)
		mockFetchLookupTableAddresses(t, rw, lookupTablePubkey, poolKeys)

		// Mock the account derivation simulations
		mockExecuteAccountDerivation(t, rw, offrampAddr.String(), []solana.PublicKey{userMessagingAccount}, []tokenTransferAccounts{ttAccount}, logicReceiver, []solana.PublicKey{lookupTablePubkey})

		txID := uuid.NewString()
		txm.On("Enqueue", mock.Anything, admin.String(), mock.MatchedBy(func(tx *solana.Transaction) bool {
			txData := tx.Message.Instructions[0].Data
			payload := txData[8:]
			var decoded ccip_offramp.Execute
			dec := ag_binary.NewBorshDecoder(payload)
			err = dec.Decode(&decoded)
			require.NoError(t, err)

			tokenIndexes := *decoded.TokenIndexes

			require.Len(t, tokenIndexes, 1)
			require.Equal(t, uint8(3), tokenIndexes[0]) // logic receiver, external execution signer, and the extra args user acccount
			return true
		}), &txID, mock.Anything, mock.AnythingOfType("utils.SetTxConfig"), mock.AnythingOfType("utils.SetTxConfig")).Return(nil).Run(func(args mock.Arguments) {
			opt1, ok := args[5].(txmutils.SetTxConfig)
			require.True(t, ok)

			opt2, ok := args[6].(txmutils.SetTxConfig)
			require.True(t, ok)

			txConfig := &txmutils.TxConfig{}
			opt1(txConfig)
			opt2(txConfig)

			require.Equal(t, false, txConfig.EstimateComputeUnitLimit)
			require.Equal(t, staticCUOverhead+700, txConfig.ComputeUnitLimit)
		}).Once()

		// stripped back report just for purposes of example
		abstractReport := ccipocr3.ExecutePluginReportSingleChain{
			Messages: []ccipocr3.Message{
				{
					Receiver: logicReceiver.Bytes(),
					Header:   ccipocr3.RampMessageHeader{SourceChainSelector: sourceChainSel},
					TokenAmounts: []ccipocr3.RampTokenAmount{
						{
							DestTokenAddress:  destTokenAddr.Bytes(),
							SourcePoolAddress: sourcePoolAddr.Bytes(),
							Amount:            ccipocr3.NewBigInt(big.NewInt(1)),
						},
					},
				},
			},
			OffchainTokenData: [][][]byte{{}},
		}

		args := ccipsolana.SVMExecCallArgs{
			ReportContext: [2][32]byte{{0x01}, {0x02}},
			Report:        make([]byte, 200), // Set report to arbitrary bytes for test. Ensure it doesn't cause tx to exceed the max solana tx size.
			Info: ccipocr3.ExecuteReportInfo{
				MerkleRoots:     []ccipocr3.MerkleRootChain{{MerkleRoot: [32]byte{}}},
				AbstractReports: []ccipocr3.ExecutePluginReportSingleChain{abstractReport},
			},
			ExtraData: ccipsolana.ExtraDataDecoded{
				ExtraArgsDecoded: map[string]any{
					"computeUnits":            uint32(500),
					"accounts":                userMessagingAccount,
					"accountIsWritableBitmap": uint64(1),
					"tokenReceiver":           tokenReceiver,
				},
				DestExecDataDecoded: []map[string]any{
					{"destGasAmount": uint32(200)},
				},
			},
		}

		submitErr := cw.SubmitTransaction(ctx, ccipconsts.ContractNameOffRamp, ccipconsts.MethodExecute, args, txID, offrampAddr.String(), nil, nil)
		require.NoError(t, submitErr)
	})

	t.Run("CCIP commit is encoded successfully and ArgsTransform is applied correctly.", func(t *testing.T) {
		// mock txm
		txm := txmMocks.NewTxManager(t)
		// initialize chain writer
		cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, ccipCWConfig)
		require.NoError(t, err)

		recentBlockHash := solana.Hash{}
		rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{Blockhash: recentBlockHash, LastValidBlockHeight: uint64(100)}}, nil).Once()

		txID := uuid.NewString()

		args := ccipsolana.SVMCommitCallArgs{
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
			var decoded ccip_offramp.Commit
			dec := ag_binary.NewBorshDecoder(payload)
			err := dec.Decode(&decoded)
			require.NoError(t, err)
			// The CCIPCommit ArgsTransform should remove the last account since no price updates were provided in the report
			require.Len(t, tx.Message.Instructions[0].Accounts, 2)
			return true
		}), &txID, mock.Anything, mock.AnythingOfType("utils.SetTxConfig")).Return(nil).Run(func(args mock.Arguments) {
			opt, ok := args[5].(txmutils.SetTxConfig)
			require.True(t, ok)
			txConfig := &txmutils.TxConfig{}
			opt(txConfig)

			require.Equal(t, true, txConfig.EstimateComputeUnitLimit)
		}).Once()

		submitErr := cw.SubmitTransaction(ctx, ccipconsts.ContractNameOffRamp, ccipconsts.MethodCommit, args, txID, offrampAddr.String(), nil, nil)
		require.NoError(t, submitErr)
	})
}

func TestChainWriter_GetTransactionStatus(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})

	ge := feemocks.NewEstimator(t)

	// mock txm
	txm := txmMocks.NewTxManager(t)

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, chainwriter.ChainWriterConfig{})
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

	ctx := t.Context()
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	ge := feemocks.NewEstimator(t)
	ge.On("BaseComputeUnitPrice").Return(uint64(100))

	// mock txm
	txm := txmMocks.NewTxManager(t)

	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	t.Run("returns valid compute unit price and non-nil data availability fee", func(t *testing.T) {
		feeComponents, err := cw.GetFeeComponents(ctx)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(100), feeComponents.ExecutionFee)
		require.Equal(t, big.NewInt(0), feeComponents.DataAvailabilityFee) // always nil for Solana
	})

	t.Run("fails if gas estimator not set", func(t *testing.T) {
		cwNoEstimator, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, nil, chainwriter.ChainWriterConfig{})
		require.NoError(t, err)
		_, err = cwNoEstimator.GetFeeComponents(ctx)
		require.Error(t, err)
	})
}

// Tests that the two versioned IDLs for the same method args encode to the same bytes
func TestChainWriter_ParsePrograms(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// mock client
	rw := clientmocks.NewReaderWriter(t)
	mc := *client.NewMultiClient(func(context.Context) (client.ReaderWriter, error) {
		return rw, nil
	})
	// mock estimator
	ge := feemocks.NewEstimator(t)
	// mock txm
	txm := txmMocks.NewTxManager(t)

	invalidConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"testIDLv1": {
				IDL: "",
			},
		},
	}

	_, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, invalidConfig)
	require.ErrorContains(t, err, "failed to unmarshal IDL for program testIDLv1 (tried both codecv2 and codec), error:")

	cwConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"testIDLv1": {
				IDL: codecv1.FetchChainWriterTestIDL(),
				Methods: map[string]chainwriter.MethodConfig{
					"TestItemArray1Type": {
						ChainSpecificName: "TestItemArray1Type",
					},
				},
			},
			"testIDLv2": {
				IDL: codecv2.FetchChainWriterTestIDL(),
				Methods: map[string]chainwriter.MethodConfig{
					"TestItemArray1Type": {
						ChainSpecificName: "test_item_array1_type",
					},
				},
			},
		},
	}

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(testutils.NewNullLogger(), mc, txm, ge, cwConfig)
	require.NoError(t, err)

	// Test v1 encoding - use codecTestUtils.TestItemAsArgs which has PascalCase field names
	testArrayV1 := [1]codecv1TestUtils.TestItemAsArgs{{
		Field:               1,
		OracleID:            2,
		OracleIDs:           [32]uint8{3},
		AccountStruct:       codecv1TestUtils.AccountStruct{},
		Accounts:            []solana.PublicKey{},
		DifferentField:      "test",
		BigField:            ag_binary.Int128{Lo: 5},
		NestedDynamicStruct: codecv1TestUtils.NestedDynamic{FixedBytes: [2]uint8{6, 7}, Inner: codecv1TestUtils.InnerDynamic{IntVal: 8, S: "inner"}},
		NestedStaticStruct:  codecv1TestUtils.NestedStatic{FixedBytes: [2]uint8{9, 10}, Inner: codecv1TestUtils.InnerStatic{IntVal: 11}},
	}}

	encodedPayloadv1, err := cw.EncodePayload(ctx, testArrayV1, chainwriter.MethodConfig{
		ChainSpecificName: "TestItemArray1Type",
	}, "testIDLv1", "TestItemArray1Type")
	require.NoError(t, err)
	require.NotNil(t, encodedPayloadv1)

	// Anchor 0.3x IDL parsing results in snake_case field names. Create structs to match these field names.
	type InnerDynamicV2 struct {
		Int_val int64 //nolint:revive // snake_case required to match Anchor IDL
		S       string
	}
	type InnerStaticV2 struct {
		Int_val int64 //nolint:revive // snake_case required to match Anchor IDL
		A       solana.PublicKey
	}
	type AccountStructV2 struct {
		Account     solana.PublicKey
		Account_str solana.PublicKey //nolint:revive // snake_case required to match Anchor IDL
	}
	type NestedDynamicV2 struct {
		Fixed_bytes [2]uint8 //nolint:revive // snake_case required to match Anchor IDL
		Inner       InnerDynamicV2
	}
	type NestedStaticV2 struct {
		Fixed_bytes [2]uint8 //nolint:revive // snake_case required to match Anchor IDL
		Inner       InnerStaticV2
	}
	type TestItemV2 struct {
		Field                 int32
		Oracle_id             uint8           //nolint:revive // snake_case required to match Anchor IDL
		Oracle_ids            [32]uint8       //nolint:revive // snake_case required to match Anchor IDL
		Account_struct        AccountStructV2 //nolint:revive // snake_case required to match Anchor IDL
		Accounts              []solana.PublicKey
		Different_field       string           //nolint:revive // snake_case required to match Anchor IDL
		Big_field             ag_binary.Int128 //nolint:revive // snake_case required to match Anchor IDL
		Nested_dynamic_struct NestedDynamicV2  //nolint:revive // snake_case required to match Anchor IDL
		Nested_static_struct  NestedStaticV2   //nolint:revive // snake_case required to match Anchor IDL
	}

	// Test v2 encoding with matching struct (same data, different field names)
	testArrayV2 := [1]TestItemV2{{
		Field:                 1,
		Oracle_id:             2,
		Oracle_ids:            [32]uint8{3},
		Account_struct:        AccountStructV2{},
		Accounts:              []solana.PublicKey{},
		Different_field:       "test",
		Big_field:             ag_binary.Int128{Lo: 5},
		Nested_dynamic_struct: NestedDynamicV2{Fixed_bytes: [2]uint8{6, 7}, Inner: InnerDynamicV2{Int_val: 8, S: "inner"}},
		Nested_static_struct:  NestedStaticV2{Fixed_bytes: [2]uint8{9, 10}, Inner: InnerStaticV2{Int_val: 11}},
	}}

	encodedPayloadv2, err := cw.EncodePayload(ctx, testArrayV2, chainwriter.MethodConfig{
		ChainSpecificName: "test_item_array1_type",
	}, "testIDLv2", "TestItemArray1Type")
	require.NoError(t, err)
	require.NotNil(t, encodedPayloadv2)

	// Both should encode to the same bytes (same data structure, just different field naming conventions)
	require.Equal(t, encodedPayloadv1, encodedPayloadv2)
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
	lookupTablePubkey := GetRandomPubKey(t)
	dataAccount := chainwriter.DataAccount{
		Version:              1,
		Administrator:        GetRandomPubKey(t),
		PendingAdministrator: GetRandomPubKey(t),
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
		addresses[i] = GetRandomPubKey(t)
	}
	return addresses
}
