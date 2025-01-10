//go:build integration

package txm

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
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
		t.Run(tc.name, func(t *testing.T) {
			tc := tc
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
			rebroadcastLogs := observer.FilterMessageSnippet("transaction expired, rebroadcasting").Len()
			rebroadcastLogs2 := observer.FilterMessageSnippet("expired tx was rebroadcasted successfully").Len()
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

func setup(t *testing.T, url string, txExpirationRebroadcast bool) (context.Context, *solanaClient.Client, *Txm, solana.PublicKey, solana.PublicKey, *observer.ObservedLogs) {
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
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(1 * time.Minute) // to get the finalized tx status

	// Initialize the Solana client and TXM
	lggr, obs := logger.TestObserved(t, zapcore.DebugLevel)
	client, err := solanaClient.NewClient(url, cfg, 2*time.Second, lggr)
	require.NoError(t, err)
	loader := utils.NewLazyLoad(func() (solanaClient.ReaderWriter, error) { return client, nil })
	txmInstance := NewTxm("localnet", loader, nil, cfg, mkey, lggr)
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

func TestTxm_Integration_Reorg(t *testing.T) {
	t.Parallel()
	t.Run("no reorg", func(t *testing.T) {
		// Setup live validator and test environment
		t.Parallel()
		url := solanaClient.SetupLocalSolNode(t)
		ctx, client, txmInstance, senderPubKey, receiverPubKey, observer := setup(t, url, true)

		// Record initial balance
		initSenderBalance, err := client.Balance(ctx, senderPubKey)
		require.NoError(t, err)
		const amount = 1 * solana.LAMPORTS_PER_SOL

		// Create, enqueue and wait for tx finalization
		txID := "no-reorg"
		tx, lastValidBlockHeight := createTransaction(ctx, t, client, senderPubKey, receiverPubKey, amount, true)
		require.NoError(t, txmInstance.Enqueue(ctx, "", tx, &txID, lastValidBlockHeight))
		require.Eventually(t, func() bool {
			status, errGetStatus := txmInstance.GetTransactionStatus(ctx, txID)
			if errGetStatus != nil {
				return false
			}
			return status == types.Finalized
		}, 60*time.Second, 1*time.Second, "Transaction should eventually reach Finalized status")

		// Verify that reorg was not detected and final balances are correct
		reorgLogs := observer.FilterMessageSnippet("re-org detected for transaction").Len()
		require.Equal(t, 0, reorgLogs, "Re-org should not occur")
		finalSenderBalance, err := client.Balance(ctx, senderPubKey)
		require.NoError(t, err)
		finalReceiverBalance, err := client.Balance(ctx, receiverPubKey)
		require.NoError(t, err)
		require.Less(t, finalSenderBalance, initSenderBalance, "Sender balance should decrease")
		require.Equal(t, amount, finalReceiverBalance, "Receiver should receive the transferred amount")
	})

	t.Run("confirmed reorg: previous tx is replaced and new one is finalized", func(t *testing.T) {
		// Start live validator and setup test environment
		t.Parallel()
		ledgerDir := t.TempDir()
		port := utils.MustRandomPort(t)
		faucetPort := utils.MustRandomPort(t)
		cmd, url := startValidator(t, ledgerDir, port, faucetPort, true)
		ctx, cl, txmInstance, senderPubKey, receiverPubKey, obs := setup(t, url, true)

		// Back up the ledger after transferring funds
		cleanLedgerBackupDir := t.TempDir()
		require.NoError(t, copyDir(ledgerDir, cleanLedgerBackupDir))
		initSenderBalance, err := cl.Balance(ctx, senderPubKey)
		require.NoError(t, err)

		// Create TX and wait for it to be confirmed
		const amount = 1 * solana.LAMPORTS_PER_SOL
		txID := "reorg-test-tx"
		tx, lastValidBlockHeight := createTransaction(ctx, t, cl, senderPubKey, receiverPubKey, amount, true)
		require.NoError(t, txmInstance.Enqueue(ctx, "", tx, &txID, lastValidBlockHeight))
		require.Eventually(t, func() bool {
			status, errGetStatus := txmInstance.GetTransactionStatus(ctx, txID)
			if errGetStatus != nil {
				return false
			}
			if status == types.Unconfirmed {
				pTx, errPtx := txmInstance.getPendingTx(txID)
				if errPtx != nil || len(pTx.signatures) == 0 {
					return false
				}

				sigStatus, errStat := cl.SignatureStatuses(ctx, pTx.signatures)
				if errStat != nil || len(sigStatus) == 0 || sigStatus[0] == nil {
					return false
				}
				return sigStatus[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed
			}
			return false
		}, 60*time.Second, 1*time.Second, "Transaction should reach Confirmed status")

		// Simulate reorg: kill current validator and restart validator with backuped ledger before the tx.
		// we want ledger as provided, omit --reset
		require.NoError(t, cmd.Process.Kill())
		_ = cmd.Wait()
		require.NoError(t, os.RemoveAll(ledgerDir))
		require.NoError(t, copyDir(cleanLedgerBackupDir, ledgerDir))
		startValidator(t, ledgerDir, port, faucetPort, false)

		// Check tx is not finalized yet and reorg is detected
		status, errGetStatus := txmInstance.GetTransactionStatus(ctx, txID)
		require.NoError(t, errGetStatus)
		require.NotEqual(t, types.Finalized, status, "tx should not be finalized after reorg")
		reorgLogs := obs.FilterMessageSnippet("re-org detected for transaction").Len()
		require.Equal(t, reorgLogs, 1, "Re-org should be detected")
		rebroadcastReorgLogs := obs.FilterMessageSnippet("re-orged tx was rebroadcasted successfully").Len()
		require.Equal(t, rebroadcastReorgLogs, 1, "re-org tx should be rebroadcasted with new blockhash")

		// Wait rebroadcasted tx to be finalized and check final balances
		require.Eventually(t, func() bool {
			finalStatus, errAgain := txmInstance.GetTransactionStatus(ctx, txID)
			if errAgain != nil {
				return false
			}
			return finalStatus == types.Finalized
		}, 120*time.Second, 5*time.Second, "tx should finalize again after reorg handling")
		finalSenderBalance, err := cl.Balance(ctx, senderPubKey)
		require.NoError(t, err)
		finalReceiverBalance, err := cl.Balance(ctx, receiverPubKey)
		require.NoError(t, err)
		require.Less(t, finalSenderBalance, initSenderBalance, "Sender balance should decrease after re-finalization")
		require.Equal(t, amount, finalReceiverBalance, "Receiver should receive transferred amount after re-finalization")
		status, errGetStatus = txmInstance.GetTransactionStatus(ctx, txID)
		require.NoError(t, errGetStatus)
		require.Equal(t, types.Finalized, status, "tx should be finalized after reorg")
	})
}

// startValidator starts a local solana-test-validator and return the cmd to control it.
func startValidator(
	t *testing.T,
	ledgerDir, port, faucetPort string,
	reset bool,
) (*exec.Cmd, string) {
	t.Helper()

	args := []string{
		"--rpc-port", port,
		"--faucet-port", faucetPort,
		"--ledger", ledgerDir,
	}
	if reset {
		args = append([]string{"--reset"}, args...)
	}

	cmd := exec.Command("solana-test-validator", args...)

	var stdErr, stdOut bytes.Buffer
	cmd.Stderr = &stdErr
	cmd.Stdout = &stdOut

	require.NoError(t, cmd.Start(), "failed to start solana-test-validator")

	// The RPC URL
	url := "http://127.0.0.1:" + port

	// Ensure validator is killed after the test finishes
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Wait until it's healthy
	client := rpc.New(url)
	require.Eventually(t, func() bool {
		out, err := client.GetHealth(context.Background())
		return err == nil && out == rpc.HealthOk
	}, 30*time.Second, 1*time.Second, "Validator should become healthy")

	return cmd, url
}

// copyDir copies the directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}
