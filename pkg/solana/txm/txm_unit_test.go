package txm_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	solanaClient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	solanatxm "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	bigmath "github.com/smartcontractkit/chainlink-common/pkg/utils/big_math"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestTxm_EstimateComputeUnitLimit(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)

	// setup mock keystore
	mkey := keyMocks.NewSimpleKeystore(t)

	// setup key
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := key.PublicKey()

	// setup receiver key
	privKeyReceiver, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKeyReceiver := privKeyReceiver.PublicKey()

	// set up txm
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	client := clientmocks.NewReaderWriter(t)
	require.NoError(t, err)
	getClient := func() (solanaClient.ReaderWriter, error) {
		return client, nil
	}
	txm := solanatxm.NewTxm("localnet", getClient, cfg, mkey, lggr)

	t.Run("successfully sets estimated compute unit limit", func(t *testing.T) {
		usedCompute := uint64(100)
		client.On("LatestBlockhash").Return(&rpc.GetLatestBlockhashResult{
			Value: &rpc.LatestBlockhashResult{
				LastValidBlockHeight: 100,
				Blockhash:            solana.Hash{},
			},
		}, nil).Once()
		client.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{
			Err:           nil,
			UnitsConsumed: &usedCompute,
		}, nil).Once()
		tx := createTx(t, client, pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)
		computeUnitLimit, err := txm.EstimateComputeUnitLimit(ctx, tx)
		require.NoError(t, err)
		usedComputeWithBuffer := bigmath.AddPercentage(new(big.Int).SetUint64(usedCompute), solanatxm.EstimateComputeUnitLimitBuffer).Uint64()
		require.Equal(t, usedComputeWithBuffer, uint64(computeUnitLimit))
	})

	t.Run("failed to simulate tx", func(t *testing.T) {
		client.On("LatestBlockhash").Return(&rpc.GetLatestBlockhashResult{
			Value: &rpc.LatestBlockhashResult{
				LastValidBlockHeight: 100,
				Blockhash:            solana.Hash{},
			},
		}, nil).Once()
		client.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("failed to simulate")).Once()
		tx := createTx(t, client, pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)
		_, err := txm.EstimateComputeUnitLimit(ctx, tx)
		require.Error(t, err)
	})

	t.Run("simulation returns error for tx", func(t *testing.T) {
		client.On("LatestBlockhash").Return(&rpc.GetLatestBlockhashResult{
			Value: &rpc.LatestBlockhashResult{
				LastValidBlockHeight: 100,
				Blockhash:            solana.Hash{},
			},
		}, nil).Once()
		client.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{
			Err: errors.New("InstructionError"),
		}, nil).Once()
		tx := createTx(t, client, pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)
		_, err := txm.EstimateComputeUnitLimit(ctx, tx)
		require.Error(t, err)
	})

	t.Run("simulation returns nil err with 0 compute unit limit", func(t *testing.T) {
		client.On("LatestBlockhash").Return(&rpc.GetLatestBlockhashResult{
			Value: &rpc.LatestBlockhashResult{
				LastValidBlockHeight: 100,
				Blockhash:            solana.Hash{},
			},
		}, nil).Once()
		client.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{
			Err: nil,
		}, nil).Once()
		tx := createTx(t, client, pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)
		computeUnitLimit, err := txm.EstimateComputeUnitLimit(ctx, tx)
		require.NoError(t, err)
		require.Equal(t, uint32(0), computeUnitLimit)
	})
}

func createTx(t *testing.T, client solanaClient.ReaderWriter, signer solana.PublicKey, sender solana.PublicKey, receiver solana.PublicKey, amt uint64) *solana.Transaction {
	// create transfer tx
	hash, err := client.LatestBlockhash()
	require.NoError(t, err)
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				amt,
				sender,
				receiver,
			).Build(),
		},
		hash.Value.Blockhash,
		solana.TransactionPayer(signer),
	)
	require.NoError(t, err)
	return tx
}
