package chainwriter_test

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	feemocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/fees/mocks"
	txmMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
)

func TestChainWriter_GetAddresses(t *testing.T) {}

func TestChainWriter_FilterLookupTableAddresses(t *testing.T) {}

func TestChainWriter_SubmitTransaction(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	rw := clientmocks.NewReaderWriter(t)
	rw.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Maybe()
	rw.On("SlotHeight", mock.Anything).Return(uint64(0), nil).Maybe()
	ge := feemocks.NewEstimator(t)

	// mock txm
	txm := txmMocks.NewTxManager(t)

	idlJSON, err := os.ReadFile("../../../contracts/target/idl/write_test.json")
	require.NoError(t, err)
	// TODO: Get IDL and address
	programID := chainwriter.GetRandomPubKey(t).String()
	programIDL := string(idlJSON)

	args := map[string]interface{}{
		"seed1":        []byte("data"),
		"lookup_table": chainwriter.GetRandomPubKey(t),
	}
	fmt.Println(args)

	adminPk, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	admin := adminPk.PublicKey()

	// TODO: Replace all random and create mocks
	cwConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"write_test": {
				Methods: map[string]chainwriter.MethodConfig{
					"initialize": {
						FromAddress: admin.String(),
						InputModifications: commoncodec.ModifiersConfig{
							&commoncodec.DropModifierConfig{
								// Drop seed1 since it shouldn't be in the instruction data
								Fields: []string{"seed1"},
							},
						},
						ChainSpecificName: "initialize",
						LookupTables: chainwriter.LookupTables{
							DerivedLookupTables: []chainwriter.DerivedLookupTable{
								{
									Name: "DerivedTable",
									Accounts: chainwriter.PDALookups{
										Name:      "DataAccountPDA",
										PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: programID},
										Seeds: []chainwriter.Lookup{
											// extract seed1 for PDA lookup
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
							StaticLookupTables: []string{chainwriter.GetRandomPubKey(t).String()},
						},
						Accounts: []chainwriter.Lookup{
							chainwriter.AccountConstant{
								Name:       "Constant",
								Address:    chainwriter.GetRandomPubKey(t).String(),
								IsSigner:   false,
								IsWritable: false,
							},
							chainwriter.AccountLookup{
								Name:       "LookupTable",
								Location:   "lookup_table",
								IsSigner:   false,
								IsWritable: false,
							},
							chainwriter.PDALookups{
								Name:      "DataAccountPDA",
								PublicKey: chainwriter.AccountConstant{Name: "WriteTest", Address: programID},
								Seeds: []chainwriter.Lookup{
									// extract seed1 for PDA lookup
									chainwriter.AccountLookup{Name: "seed1", Location: "seed1"},
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
						},
					},
				},
				IDL: programIDL,
			},
		},
	}

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(rw, txm, ge, cwConfig)
	require.NoError(t, err)

	t.Run("fails with invalid ABI", func(t *testing.T) {
		invalidCWConfig := chainwriter.ChainWriterConfig{
			Programs: map[string]chainwriter.ProgramConfig{
				"write_test": {
					Methods: map[string]chainwriter.MethodConfig{
						"invalid": {
							ChainSpecificName: "invalid",
						},
					},
					IDL: "",
				},
			},
		}

		_, err := chainwriter.NewSolanaChainWriterService(rw, txm, ge, invalidCWConfig)
		require.Error(t, err)
	})

	t.Run("Submits transaction successfully", func(t *testing.T) {
		rw.On("GetAccountInfoWithOpts", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.GetAccountInfoResult{
			RPCContext: rpc.RPCContext{},
			Value:      &rpc.Account{},
		}, nil).Maybe()
		args := map[string]interface{}{
			"lookupTable": chainwriter.GetRandomPubKey(t).String(),
			"seed1":       []byte("data"),
		}
		err := cw.SubmitTransaction(ctx, "write_test", "initialize", args, "1", programID, nil, nil)
		fmt.Println(err)
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
	cw, err := chainwriter.NewSolanaChainWriterService(rw, txm, ge, chainwriter.ChainWriterConfig{})
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

	cw, err := chainwriter.NewSolanaChainWriterService(rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	t.Run("returns valid compute unit price", func(t *testing.T) {
		feeComponents, err := cw.GetFeeComponents(ctx)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(100), feeComponents.ExecutionFee)
		require.Nil(t, feeComponents.DataAvailabilityFee) // always nil for Solana
	})

	t.Run("fails if gas estimator not set", func(t *testing.T) {
		cwNoEstimator, err := chainwriter.NewSolanaChainWriterService(rw, txm, nil, chainwriter.ChainWriterConfig{})
		require.NoError(t, err)
		_, err = cwNoEstimator.GetFeeComponents(ctx)
		require.Error(t, err)
	})
}
