package chainwriter_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	relayconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	feemocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/fees/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
)

func TestChainWriter_GetAddresses(t *testing.T) {}

func TestChainWriter_FilterLookupTableAddresses(t *testing.T) {}

func TestChainWriter_SubmitTransaction(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	// Retain transactions after finality or error to maintain their status in memory
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(5 * time.Second)
	// Disable bumping to avoid issues with send tx mocking
	cfg.Chain.FeeBumpPeriod = relayconfig.MustNewDuration(0 * time.Second)
	rw := clientmocks.NewReaderWriter(t)
	rw.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Maybe()
	rw.On("SlotHeight", mock.Anything).Return(uint64(0), nil).Maybe()
	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return rw, nil })
	ge := feemocks.NewEstimator(t)
	// mock solana keystore
	keystore := keyMocks.NewSimpleKeystore(t)
	keystore.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()

	// initialize and start TXM
	txm := txm.NewTxm(uuid.NewString(), loader, nil, cfg, keystore, lggr)
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

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
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	// Retain transactions after finality or error to maintain their status in memory
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(5 * time.Second)
	// Disable bumping to avoid issues with send tx mocking
	cfg.Chain.FeeBumpPeriod = relayconfig.MustNewDuration(0 * time.Second)
	rw := clientmocks.NewReaderWriter(t)
	rw.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Maybe()
	rw.On("SlotHeight", mock.Anything).Return(uint64(0), nil).Maybe()
	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return rw, nil })
	ge := feemocks.NewEstimator(t)
	// mock solana keystore
	keystore := keyMocks.NewSimpleKeystore(t)
	keystore.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()

	// initialize and start TXM
	txm := txm.NewTxm(uuid.NewString(), loader, nil, cfg, keystore, lggr)
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	// initialize chain writer
	cw, err := chainwriter.NewSolanaChainWriterService(rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)

	computeUnitLimitDefault := fees.ComputeUnitLimit(cfg.ComputeUnitLimitDefault())

	// mock signature statuses calls
	statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}
	rw.On("SignatureStatuses", mock.Anything, mock.AnythingOfType("[]solana.Signature")).Return(
		func(_ context.Context, sigs []solana.Signature) (out []*rpc.SignatureStatusesResult) {
			for i := range sigs {
				get, exists := statuses[sigs[i]]
				if !exists {
					out = append(out, nil)
					continue
				}
				out = append(out, get())
			}
			return out
		}, nil,
	)

	t.Run("returns unknown with error if ID not found", func(t *testing.T) {
		status, err := cw.GetTransactionStatus(ctx, uuid.NewString())
		require.Error(t, err)
		require.Equal(t, types.Unknown, status)
	})

	t.Run("returns pending when transaction is broadcasted", func(t *testing.T) {
		tx, signed := getTx(t, 1, keystore)
		signedTx := signed(0, true, computeUnitLimitDefault)
		for _, ins := range signedTx.Message.Instructions {
			if cuprice, err := fees.ParseComputeUnitPrice(ins.Data); err == nil {
				t.Log("compute unit price", cuprice)
			}
		}
		sig := randomSignature(t)
		rw.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		rw.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()

		// mock transaction in broadcasted state
		var wg sync.WaitGroup
		wg.Add(1)
		count := 0
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			defer func() { count++ }()
			if count == 0 {
				wg.Done()
			}
			return nil
		}

		txID := uuid.NewString()
		err = txm.Enqueue(ctx, uuid.NewString(), tx, &txID)
		require.NoError(t, err)
		// wait till transaction is broadcasted
		wg.Wait()
		// wait for next confirm cycle to ensure transaction had enough time to update in storage
		time.Sleep(cfg.ConfirmPollPeriod())

		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Pending, status)
	})

	t.Run("returns unconfirmed when transaction is processed", func(t *testing.T) {
		tx, signed := getTx(t, 2, keystore)
		sig := randomSignature(t)
		rw.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		rw.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()

		// mock transaction in processed state
		var wg sync.WaitGroup
		wg.Add(1)
		count := 0
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			defer func() { count++ }()
			if count == 0 {
				wg.Done()
			}
			return &rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusProcessed}
		}

		txID := uuid.NewString()
		err = txm.Enqueue(ctx, uuid.NewString(), tx, &txID)
		require.NoError(t, err)
		// wait till transaction is processed
		wg.Wait()
		// wait for next confirm cycle to ensure transaction had enough time to update in storage
		time.Sleep(cfg.ConfirmPollPeriod())

		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Unconfirmed, status)
	})

	t.Run("returns unconfirmed when transaction is confirmed", func(t *testing.T) {
		tx, signed := getTx(t, 3, keystore)
		sig := randomSignature(t)
		rw.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		rw.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()

		// mock transaction in processed state
		var wg sync.WaitGroup
		wg.Add(1)
		count := 0
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			defer func() { count++ }()
			if count == 0 {
				wg.Done()
			}
			return &rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusConfirmed}
		}

		txID := uuid.NewString()
		err = txm.Enqueue(ctx, uuid.NewString(), tx, &txID)
		require.NoError(t, err)
		// wait till transaction is confirmed
		wg.Wait()
		// wait for next confirm cycle to ensure transaction had enough time to update in storage
		time.Sleep(cfg.ConfirmPollPeriod())

		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Unconfirmed, status)
	})

	t.Run("returns finalized when transaction is finalized", func(t *testing.T) {
		tx, signed := getTx(t, 4, keystore)
		sig := randomSignature(t)
		rw.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		rw.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()

		// mock transaction in processed state
		var wg sync.WaitGroup
		wg.Add(1)
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			defer wg.Done()
			return &rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusFinalized}
		}

		txID := uuid.NewString()
		err = txm.Enqueue(ctx, uuid.NewString(), tx, &txID)
		require.NoError(t, err)
		// wait till transaction is finalized
		wg.Wait()
		// wait for next confirm cycle to ensure transaction had enough time to update in storage
		time.Sleep(cfg.ConfirmPollPeriod())

		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)
	})

	t.Run("returns failed when error encountered", func(t *testing.T) {
		tx, signed := getTx(t, 5, keystore)
		sig := randomSignature(t)
		var wg sync.WaitGroup
		wg.Add(1)
		rw.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		rw.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
			wg.Done()
		}).Return(&rpc.SimulateTransactionResult{
			Err: "FAIL",
		}, nil).Maybe()

		// mock transaction in processed state
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			return nil
		}

		txID := uuid.NewString()
		err = txm.Enqueue(ctx, uuid.NewString(), tx, &txID)
		require.NoError(t, err)
		// wait till transaction is finalized
		wg.Wait()

		status, err := cw.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Failed, status)
	})
}

func TestChainWriter_GetFeeComponents(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	cfg := config.NewDefault()
	rw := clientmocks.NewReaderWriter(t)
	ge := feemocks.NewEstimator(t)
	ge.On("BaseComputeUnitPrice").Return(uint64(100))
	cw := setupChainWriter(t, cfg, rw, ge)
	t.Run("returns valid compute unit price", func(t *testing.T) {
		feeComponents, err := cw.GetFeeComponents(ctx)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(100), feeComponents.ExecutionFee)
		require.Nil(t, feeComponents.DataAvailabilityFee) // always nil for Solana
	})

	t.Run("fails if gas estimator not set", func(t *testing.T) {
		cwNoEstimator := setupChainWriter(t, cfg, rw, nil)
		_, err := cwNoEstimator.GetFeeComponents(ctx)
		require.Error(t, err)
	})
}

func setupChainWriter(t *testing.T, cfg *config.TOMLConfig, rw client.ReaderWriter, ge fees.Estimator) *chainwriter.SolanaChainWriterService {
	ctx := tests.Context(t)
	lggr := logger.Test(t)
	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return rw, nil })
	// mock solana keystore
	keystore := keyMocks.NewSimpleKeystore(t)
	keystore.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil).Maybe()
	// initialize and start TXM
	txm := txm.NewTxm(uuid.NewString(), loader, nil, cfg, keystore, lggr)
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	cw, err := chainwriter.NewSolanaChainWriterService(rw, txm, ge, chainwriter.ChainWriterConfig{})
	require.NoError(t, err)
	return cw
}

func randomSignature(t *testing.T) solana.Signature {
	// make random signature
	sig := make([]byte, 64)
	_, err := rand.Read(sig)
	require.NoError(t, err)

	return solana.SignatureFromBytes(sig)
}

// create placeholder transaction and returns func for signed tx with fee
func getTx(t *testing.T, val uint64, keystore txm.SimpleKeystore) (*solana.Transaction, func(fees.ComputeUnitPrice, bool, fees.ComputeUnitLimit) *solana.Transaction) {
	pubkey := solana.PublicKey{}

	// create transfer tx
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				val,
				pubkey,
				pubkey,
			).Build(),
		},
		solana.Hash{},
		solana.TransactionPayer(pubkey),
	)
	require.NoError(t, err)

	base := *tx // tx to send to txm, txm will add fee & sign

	return &base, func(price fees.ComputeUnitPrice, addLimit bool, limit fees.ComputeUnitLimit) *solana.Transaction {
		tx := base
		// add fee parameters
		require.NoError(t, fees.SetComputeUnitPrice(&tx, price))
		if addLimit {
			require.NoError(t, fees.SetComputeUnitLimit(&tx, limit)) // default
		}

		// sign tx
		txMsg, err := tx.Message.MarshalBinary()
		require.NoError(t, err)
		sigBytes, err := keystore.Sign(tests.Context(t), pubkey.String(), txMsg)
		require.NoError(t, err)
		var finalSig [64]byte
		copy(finalSig[:], sigBytes)
		tx.Signatures = append(tx.Signatures, finalSig)
		return &tx
	}
}
