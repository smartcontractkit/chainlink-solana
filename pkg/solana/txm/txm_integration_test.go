//go:build integration

package txm_test

import (
	"context"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services/servicetest"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	relayconfig "github.com/smartcontractkit/chainlink-common/pkg/config"

	solanaClient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
)

func TestTxm_Integration_ExpirationRebroadcast(t *testing.T) {
	t.Parallel()
	url := solanaClient.SetupLocalSolNode(t) // live validator

	type TestCase struct {
		name                    string
		txExpirationRebroadcast bool
		useValidBlockHash       bool
		expectRebroadcast       bool
		expectTransactionStatus types.TransactionStatus
	}

	testCases := []TestCase{
		{
			name:                    "WithRebroadcast",
			txExpirationRebroadcast: true,
			useValidBlockHash:       false,
			expectRebroadcast:       true,
			expectTransactionStatus: types.Finalized,
		},
		{
			name:                    "WithoutRebroadcast",
			txExpirationRebroadcast: false,
			useValidBlockHash:       false,
			expectRebroadcast:       false,
			expectTransactionStatus: types.Failed,
		},
		{
			name:                    "ConfirmedBeforeRebroadcast",
			txExpirationRebroadcast: true,
			useValidBlockHash:       true,
			expectRebroadcast:       false,
			expectTransactionStatus: types.Finalized,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, client, txmInstance, senderPubKey, receiverPubKey, observer := setup(t, url, tc.txExpirationRebroadcast)

			// Record initial balance
			initSenderBalance, err := client.Balance(ctx, senderPubKey)
			require.NoError(t, err)
			const amount = 1 * solana.LAMPORTS_PER_SOL

			// Create and enqueue tx
			txID := tc.name
			tx, lastValidBlockHeight := createTransaction(ctx, t, client, senderPubKey, receiverPubKey, amount, tc.useValidBlockHash)
			require.NoError(t, txmInstance.Enqueue(ctx, "", tx, &txID, lastValidBlockHeight))

			// Wait for the transaction to reach the expected status
			require.Eventually(t, func() bool {
				status, statusErr := txmInstance.GetTransactionStatus(ctx, txID)
				if statusErr != nil {
					return false
				}
				return status == tc.expectTransactionStatus
			}, 60*time.Second, 1*time.Second, "Transaction should eventually reach expected status")

			// Verify balances
			finalSenderBalance, err := client.Balance(ctx, senderPubKey)
			require.NoError(t, err)
			finalReceiverBalance, err := client.Balance(ctx, receiverPubKey)
			require.NoError(t, err)

			if tc.expectTransactionStatus == types.Finalized {
				require.Less(t, finalSenderBalance, initSenderBalance, "Sender balance should decrease")
				require.Equal(t, amount, finalReceiverBalance, "Receiver should receive the transferred amount")
			} else {
				require.Equal(t, initSenderBalance, finalSenderBalance, "Sender balance should remain the same")
				require.Equal(t, uint64(0), finalReceiverBalance, "Receiver should not receive any funds")
			}

			// Verify rebroadcast logs
			rebroadcastLogs := observer.FilterMessageSnippet("rebroadcast transaction sent").Len()
			rebroadcastLogs2 := observer.FilterMessageSnippet("transaction expired, rebroadcasting").Len()
			if tc.expectRebroadcast {
				require.Equal(t, 1, rebroadcastLogs, "Expected rebroadcast log message not found")
				require.Equal(t, 1, rebroadcastLogs2, "Expected rebroadcast log message not found")
			} else {
				require.Equal(t, 0, rebroadcastLogs, "Rebroadcast should not occur")
				require.Equal(t, 0, rebroadcastLogs2, "Rebroadcast should not occur")
			}
		})
	}
}

func setup(t *testing.T, url string, txExpirationRebroadcast bool) (context.Context, *solanaClient.Client, *txm.Txm, solana.PublicKey, solana.PublicKey, *observer.ObservedLogs) {
	ctx := tests.Context(t)

	// Generate sender and receiver keys and fund sender account
	senderKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	senderPubKey := senderKey.PublicKey()
	receiverKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	receiverPubKey := receiverKey.PublicKey()
	solanaClient.FundTestAccounts(t, []solana.PublicKey{senderPubKey}, url)

	// Set up mock keystore with sender key
	mkey := keyMocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, senderPubKey.String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
		sig, _ := senderKey.Sign(data)
		return sig[:]
	}, nil)

	// Set configs
	cfg := config.NewDefault()
	cfg.Chain.TxExpirationRebroadcast = &txExpirationRebroadcast
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(10 * time.Second) // to get the finalized tx status

	// Initialize the Solana client and TXM
	lggr, obs := logger.TestObserved(t, zapcore.DebugLevel)
	client, err := solanaClient.NewClient(url, cfg, 2*time.Second, lggr)
	require.NoError(t, err)
	loader := utils.NewLazyLoad(func() (solanaClient.ReaderWriter, error) { return client, nil })
	txmInstance := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)
	servicetest.Run(t, txmInstance)

	return ctx, client, txmInstance, senderPubKey, receiverPubKey, obs
}

// createTransaction is a helper function to create a transaction based on the test case.
func createTransaction(ctx context.Context, t *testing.T, client *solanaClient.Client, senderPubKey, receiverPubKey solana.PublicKey, amount uint64, useValidBlockHash bool) (*solana.Transaction, uint64) {
	var blockhash solana.Hash
	var lastValidBlockHeight uint64

	if useValidBlockHash {
		// Get a valid recent blockhash
		recentBlockHashResult, err := client.LatestBlockhash(ctx)
		require.NoError(t, err)
		blockhash = recentBlockHashResult.Value.Blockhash
		lastValidBlockHeight = recentBlockHashResult.Value.LastValidBlockHeight
	} else {
		// Use empty blockhash to simulate expiration
		blockhash = solana.Hash{}
		lastValidBlockHeight = 0
	}

	// Create the transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				amount,
				senderPubKey,
				receiverPubKey,
			).Build(),
		},
		blockhash,
		solana.TransactionPayer(senderPubKey),
	)
	require.NoError(t, err)

	return tx, lastValidBlockHeight
}
