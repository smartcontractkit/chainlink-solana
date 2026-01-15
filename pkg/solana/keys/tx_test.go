package keys_test

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	commonks "github.com/smartcontractkit/chainlink-common/keystore"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	solcfg "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	solanaks "github.com/smartcontractkit/chainlink-solana/pkg/solana/keys"
	solanatesting "github.com/smartcontractkit/chainlink-solana/pkg/solana/testing"
)

func TestTxKey(t *testing.T) {
	storage := commonks.NewMemoryStorage()
	ctx := t.Context()
	ks, err := commonks.LoadKeystore(ctx, storage, "test-password", commonks.WithScryptParams(commonks.FastScryptParams))
	require.NoError(t, err)
	testKey, err := solanaks.CreateTxKey(ks, "test-tx-key")
	require.NoError(t, err)
	testKey2, err := solanaks.CreateTxKey(ks, "test-tx-key2")
	require.NoError(t, err)

	url := solanatesting.SetupLocalSolNode(t)
	solanatesting.FundTestAccounts(t, []solana.PublicKey{testKey.Address()}, url)
	cfg := solcfg.NewDefault()
	c, err := client.NewClient(url, cfg, 5*time.Second, logger.Test(t))
	require.NoError(t, err)

	bal, err := c.Balance(ctx, testKey.Address())
	require.NoError(t, err)
	require.Equal(t, 100*solana.LAMPORTS_PER_SOL, bal)

	bal2, err := c.Balance(ctx, testKey2.Address())
	require.NoError(t, err)
	require.Equal(t, 0*solana.LAMPORTS_PER_SOL, bal2)

	hash, err := c.LatestBlockhash(ctx)
	require.NoError(t, err)

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				solana.LAMPORTS_PER_SOL,
				testKey.Address(),
				testKey2.Address(),
			).Build(),
		},
		hash.Value.Blockhash,
		solana.TransactionPayer(testKey.Address()),
	)
	require.NoError(t, err)
	_, err = testKey.SignTx(ctx, solanaks.SignTxRequest{
		Tx: tx,
	})
	require.NoError(t, err)

	sig, err := c.SendTx(ctx, tx)
	require.NoError(t, err)

	// Wait for the transaction to be confirmed.
	tests.AssertEventually(t, func() bool {
		status, err2 := c.SignatureStatuses(ctx, []solana.Signature{sig})
		if err2 != nil {
			return false
		}
		if len(status) == 0 || status[0] == nil {
			return false
		}
		return status[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
			status[0].ConfirmationStatus == rpc.ConfirmationStatusFinalized
	})

	// Verify balances.
	fee, err := c.GetFeeForMessage(ctx, tx.Message.ToBase64())
	require.NoError(t, err)

	bal, err = c.Balance(ctx, testKey.Address())
	require.NoError(t, err)
	require.Equal(t, 99*solana.LAMPORTS_PER_SOL-fee, bal)

	bal2, err = c.Balance(ctx, testKey2.Address())
	require.NoError(t, err)
	require.Equal(t, solana.LAMPORTS_PER_SOL, bal2)

	// Test filtering: create a non-solana key and verify it's filtered out
	nonSolanaKey, err := ks.CreateKeys(ctx, commonks.CreateKeysRequest{
		Keys: []commonks.CreateKeyRequest{
			{KeyName: "evm/tx/non-solana-key", KeyType: commonks.ECDSA_S256},
		},
	})
	require.NoError(t, err)
	require.Len(t, nonSolanaKey.Keys, 1)

	// Empty names will return only solana keys (filtered).
	keys, err := solanaks.GetTxKeys(ctx, ks, []string{})
	require.NoError(t, err)
	require.Len(t, keys, 2) // Only testKey and testKey2, not the non-solana key
	keyAddresses := make([]string, len(keys))
	for i, k := range keys {
		keyAddresses[i] = k.Address().String()
	}
	require.Contains(t, keyAddresses, testKey.Address().String())
	require.Contains(t, keyAddresses, testKey2.Address().String())
	// Verify non-solana key is filtered out (ECDSA key won't be in the results)

	// Admin operation will invalidate the keys.
	_, err = ks.DeleteKeys(ctx, commonks.DeleteKeysRequest{
		KeyNames: []string{testKey.KeyPath().String(), testKey2.KeyPath().String(), "evm/tx/non-solana-key"},
	})
	require.NoError(t, err)

	// Empty names will return all keys.
	keys, err = solanaks.GetTxKeys(ctx, ks, []string{})
	require.NoError(t, err)
	require.Empty(t, keys)

	// Signing will now error.
	_, err = testKey.SignTx(ctx, solanaks.SignTxRequest{
		Tx: tx,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, commonks.ErrKeyNotFound)
}
