package txm

import (
	"math"
	"sync"
	"testing"
	"time"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	ksmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

func newTransferTx(t *testing.T, payer solanaGo.PublicKey) *solanaGo.Transaction {
	t.Helper()
	tx, err := solanaGo.NewTransaction(
		[]solanaGo.Instruction{system.NewTransferInstruction(1, payer, solanaGo.PublicKey{2}).Build()},
		solanaGo.Hash{3},
		solanaGo.TransactionPayer(payer),
	)
	require.NoError(t, err)
	return tx
}

func TestToV1Tx(t *testing.T) {
	t.Parallel()
	payer := solanaGo.PublicKey{1}

	t.Run("converts legacy tx without mutating it", func(t *testing.T) {
		tx := newTransferTx(t, payer)
		original := utils.DeepCopyTx(*tx)

		v1Tx, err := toV1Tx(*tx)
		require.NoError(t, err)
		require.Equal(t, solanaGo.MessageVersionV1, v1Tx.Message.GetVersion())
		require.Equal(t, tx.Message.Instructions, v1Tx.Message.Instructions)
		require.Equal(t, original, *tx)
		require.Equal(t, solanaGo.MessageVersionLegacy, tx.Message.GetVersion())
	})

	t.Run("rejects address lookup tables", func(t *testing.T) {
		tx := newTransferTx(t, payer)
		tx.Message.SetAddressTableLookups([]solanaGo.MessageAddressTableLookup{{AccountKey: solanaGo.PublicKey{9}, WritableIndexes: []uint8{0}}})
		_, err := toV1Tx(*tx)
		require.ErrorContains(t, err, "address table lookups")
	})

	t.Run("rejects compute budget instructions", func(t *testing.T) {
		tx := newTransferTx(t, payer)
		require.NoError(t, fees.SetComputeUnitPrice(tx, 10))
		_, err := toV1Tx(*tx)
		require.ErrorContains(t, err, "ComputeBudget instructions are not supported")
	})
}

func TestApplyComputeBudget(t *testing.T) {
	t.Parallel()

	t.Run("legacy tx gets ComputeBudget instructions", func(t *testing.T) {
		tx := newTransferTx(t, solanaGo.PublicKey{1})
		originalIxs := len(tx.Message.Instructions)
		require.NoError(t, applyComputeBudget(tx, 100_000, 1_000))
		require.Greater(t, len(tx.Message.Instructions), originalIxs)
		require.Nil(t, tx.Message.TransactionConfig.ComputeUnitLimit)
		require.Nil(t, tx.Message.TransactionConfig.PriorityFee)
	})

	t.Run("v1 tx gets inline TransactionConfig", func(t *testing.T) {
		tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
		require.NoError(t, err)
		originalIxs := tx.Message.Instructions
		require.NoError(t, applyComputeBudget(&tx, 100_000, 1_000))
		require.Equal(t, originalIxs, tx.Message.Instructions)
		require.Equal(t, uint32(100_000), *tx.Message.TransactionConfig.ComputeUnitLimit)
		require.Equal(t, uint64(100), *tx.Message.TransactionConfig.PriorityFee)
		require.Equal(t, uint32(MaxLoadedAccountsDataSizeLimit), *tx.Message.TransactionConfig.LoadedAccountsDataSizeLimit)
	})
}

func TestPrepareTxForSimulation(t *testing.T) {
	t.Parallel()

	t.Run("v1 sets max CU and data-size limits", func(t *testing.T) {
		tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
		require.NoError(t, err)
		require.NoError(t, prepareTxForSimulation(&tx))
		require.Equal(t, uint32(MaxComputeUnitLimit), *tx.Message.TransactionConfig.ComputeUnitLimit)
		require.Equal(t, uint32(MaxLoadedAccountsDataSizeLimit), *tx.Message.TransactionConfig.LoadedAccountsDataSizeLimit)
	})

	t.Run("v1 preserves an existing data-size limit", func(t *testing.T) {
		tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
		require.NoError(t, err)
		tx.Message.TransactionConfig = tx.Message.TransactionConfig.WithLoadedAccountsDataSizeLimit(1_000_000)
		require.NoError(t, prepareTxForSimulation(&tx))
		require.Equal(t, uint32(MaxComputeUnitLimit), *tx.Message.TransactionConfig.ComputeUnitLimit)
		require.Equal(t, uint32(1_000_000), *tx.Message.TransactionConfig.LoadedAccountsDataSizeLimit)
	})
}

func TestV1PriorityFee(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint64(0), v1PriorityFee(0, 200_000))
	require.Equal(t, uint64(1), v1PriorityFee(1, 200_000))                                       // 0.2 lamports rounds up
	require.Equal(t, uint64(200), v1PriorityFee(1_000, 200_000))                                 // exact
	require.Equal(t, uint64(201), v1PriorityFee(1_001, 200_000))                                 // 200.2 rounds up
	require.Equal(t, uint64(math.MaxUint64), v1PriorityFee(math.MaxUint64, MaxComputeUnitLimit)) // saturates
}

func TestTxm_BuildTx_V1(t *testing.T) {
	t.Parallel()

	ks := ksmocks.NewSimpleKeystore(t)
	ks.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(make([]byte, 64), nil)
	loader := utils.NewStaticLoader[client.ReaderWriter](clientmocks.NewReaderWriter(t))
	txm, err := NewTxm("buildtx_v1_test", loader, nil, config.NewDefault(), ks, logger.Test(t))
	require.NoError(t, err)

	v1Tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
	require.NoError(t, err)
	v1Tx.Message.TransactionConfig = v1Tx.Message.TransactionConfig.WithHeapSize(64 * 1024)
	original := utils.DeepCopyTx(v1Tx)

	cfg := txmutils.TxConfig{ComputeUnitLimit: 100_000, BaseComputeUnitPrice: 1_000, ComputeUnitPriceMin: 0, ComputeUnitPriceMax: 1_000_000}
	msg := pendingTx{tx: v1Tx, cfg: cfg}

	built, err := txm.buildTx(t.Context(), msg, 0)
	require.NoError(t, err)
	require.Equal(t, original, msg.tx, "buildTx mutated the caller's transaction")
	require.Equal(t, solanaGo.MessageVersionV1, built.Message.GetVersion())
	require.Equal(t, v1Tx.Message.Instructions, built.Message.Instructions, "no ComputeBudget instructions should be added")
	require.Equal(t, v1Tx.Message.AccountKeys, built.Message.AccountKeys)
	require.Len(t, built.Signatures, 1)

	txCfg := built.Message.TransactionConfig
	require.NotNil(t, txCfg.ComputeUnitLimit)
	require.Equal(t, uint32(100_000), *txCfg.ComputeUnitLimit)
	require.NotNil(t, txCfg.PriorityFee)
	require.Equal(t, uint64(100), *txCfg.PriorityFee)
	require.NotNil(t, txCfg.HeapSize, "caller provided config values should be preserved")
	require.Equal(t, uint32(64*1024), *txCfg.HeapSize)
	require.NotNil(t, txCfg.LoadedAccountsDataSizeLimit)
	require.Equal(t, uint32(MaxLoadedAccountsDataSizeLimit), *txCfg.LoadedAccountsDataSizeLimit)

	wire, err := built.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, byte(0x81), wire[0])

	t.Run("fee bump increases priority fee", func(t *testing.T) {
		bumped, err := txm.buildTx(t.Context(), msg, 1)
		require.NoError(t, err)
		require.Greater(t, *bumped.Message.TransactionConfig.PriorityFee, *txCfg.PriorityFee)
		require.Equal(t, *txCfg.ComputeUnitLimit, *bumped.Message.TransactionConfig.ComputeUnitLimit)
	})

	t.Run("falls back to default compute unit limit", func(t *testing.T) {
		noLimit := msg
		noLimit.cfg.ComputeUnitLimit = 0
		built, err := txm.buildTx(t.Context(), noLimit, 0)
		require.NoError(t, err)
		require.Equal(t, uint32(DefaultInstructionComputeUnitLimit), *built.Message.TransactionConfig.ComputeUnitLimit)
		require.Equal(t, uint64(200), *built.Message.TransactionConfig.PriorityFee)
	})
}

func TestTxm_EstimateComputeUnitLimit_V1(t *testing.T) {
	t.Parallel()

	ks := ksmocks.NewSimpleKeystore(t)
	ks.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(make([]byte, 64), nil)
	mc := clientmocks.NewReaderWriter(t)
	txm, err := NewTxm("estimate_v1_test", utils.NewStaticLoader[client.ReaderWriter](mc), nil, config.NewDefault(), ks, logger.Test(t))
	require.NoError(t, err)

	v1Tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
	require.NoError(t, err)

	used := uint64(1_000)
	mc.On("SimulateTx", mock.Anything, mock.IsType(&solanaGo.Transaction{}), mock.Anything).Run(func(args mock.Arguments) {
		simTx := args.Get(1).(*solanaGo.Transaction)
		assert.Equal(t, solanaGo.MessageVersionV1, simTx.Message.GetVersion())
		assert.Equal(t, v1Tx.Message.Instructions, simTx.Message.Instructions)
		if assert.NotNil(t, simTx.Message.TransactionConfig.ComputeUnitLimit) {
			assert.Equal(t, uint32(MaxComputeUnitLimit), *simTx.Message.TransactionConfig.ComputeUnitLimit)
		}
		if assert.NotNil(t, simTx.Message.TransactionConfig.LoadedAccountsDataSizeLimit) {
			assert.Equal(t, uint32(MaxLoadedAccountsDataSizeLimit), *simTx.Message.TransactionConfig.LoadedAccountsDataSizeLimit)
		}
	}).Return(&rpc.SimulateTransactionResult{UnitsConsumed: &used}, nil).Once()

	limit, err := txm.EstimateComputeUnitLimit(t.Context(), &v1Tx, "")
	require.NoError(t, err)
	require.Equal(t, uint32(1_100), limit)
	require.Nil(t, v1Tx.Message.TransactionConfig.ComputeUnitLimit, "estimation must not mutate the tx")
}

func TestTxm_Enqueue_V1(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	ks := ksmocks.NewSimpleKeystore(t)
	ks.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return(make([]byte, 64), nil)
	mc := clientmocks.NewReaderWriter(t)
	mc.On("SignatureStatuses", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mc.On("GetLatestBlockHeight", mock.Anything).Return(uint64(1), nil).Maybe()
	mc.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()

	cfg := config.NewDefault()
	txm, err := NewTxm("enqueue_v1_test", utils.NewStaticLoader[client.ReaderWriter](mc), nil, cfg, ks, logger.Test(t))
	require.NoError(t, err)

	v1Tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
	require.NoError(t, err)
	require.ErrorContains(t, txm.Enqueue(ctx, "", &v1Tx, nil, 100), "not started")

	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	t.Run("sends v1 tx", func(t *testing.T) {
		tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
		require.NoError(t, err)
		original := utils.DeepCopyTx(tx)

		var once sync.Once
		sent := make(chan solanaGo.Transaction, 1)
		mc.On("SendTx", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			once.Do(func() { sent <- *args.Get(1).(*solanaGo.Transaction) })
		}).Return(solanaGo.Signature{4}, nil)

		txID := uuid.NewString()
		require.NoError(t, txm.Enqueue(ctx, "", &tx, &txID, 100))
		require.Equal(t, original, tx, "Enqueue must not mutate the caller's tx")

		select {
		case sentTx := <-sent:
			require.Equal(t, solanaGo.MessageVersionV1, sentTx.Message.GetVersion())
			require.Equal(t, tx.Message.Instructions, sentTx.Message.Instructions)
			require.NotNil(t, sentTx.Message.TransactionConfig.ComputeUnitLimit)
			require.Equal(t, cfg.ComputeUnitLimitDefault(), *sentTx.Message.TransactionConfig.ComputeUnitLimit)
			require.NotNil(t, sentTx.Message.TransactionConfig.PriorityFee)
			require.NotNil(t, sentTx.Message.TransactionConfig.LoadedAccountsDataSizeLimit)
			_, err := sentTx.MarshalBinary()
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("tx was not sent")
		}

		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, commontypes.Pending, status)
	})

	t.Run("rejects invalid v1 tx", func(t *testing.T) {
		require.ErrorContains(t, txm.Enqueue(ctx, "", nil, nil, 100), "tx is nil pointer")

		tx, err := toV1Tx(*newTransferTx(t, solanaGo.PublicKey{1}))
		require.NoError(t, err)
		require.NoError(t, fees.SetComputeUnitLimit(&tx, 1_000))
		require.ErrorContains(t, txm.Enqueue(ctx, "", &tx, nil, 100), "ComputeBudget instructions are not supported")
	})
}
