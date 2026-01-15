package keys_test

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/keystore"
	solanaks "github.com/smartcontractkit/chainlink-solana/pkg/solana/keys"
)

func TestTxKeyCoreKeystore(t *testing.T) {
	ctx := context.Background()
	storage := keystore.NewMemoryStorage()
	ks, err := keystore.LoadKeystore(ctx, storage, "test-password", keystore.WithScryptParams(keystore.FastScryptParams))
	require.NoError(t, err)

	coreKs := solanaks.NewTxKeyCoreKeystore(ks)
	accounts, err := coreKs.Accounts(ctx)
	require.NoError(t, err)
	require.Empty(t, accounts)

	txKey, err := solanaks.CreateTxKey(ks, "key1")
	require.NoError(t, err)

	keys, err := ks.GetKeys(ctx, keystore.GetKeysRequest{
		KeyNames: []string{txKey.KeyPath().String()},
	})
	require.NoError(t, err)
	require.Len(t, keys.Keys, 1)

	// Test Accounts() returns the address
	accounts, err = coreKs.Accounts(ctx)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, txKey.Address().String(), accounts[0])

	// Test Sign() with address string
	data := []byte("test data to sign")
	signature, err := coreKs.Sign(ctx, txKey.Address().String(), data)
	require.NoError(t, err)
	require.NotNil(t, signature)
	require.Len(t, signature, ed25519.SignatureSize)

	// Verify signature
	resp, err := ks.Verify(ctx, keystore.VerifyRequest{
		KeyType:   keystore.Ed25519,
		PublicKey: keys.Keys[0].KeyInfo.PublicKey,
		Data:      data,
		Signature: signature,
	})
	require.NoError(t, err)
	require.True(t, resp.Valid)

	// Make sure the cache populated (check via second sign)
	// Sign again with cached path
	signature2, err := coreKs.Sign(ctx, txKey.Address().String(), data)
	require.NoError(t, err)
	require.NotNil(t, signature2)
	require.Equal(t, signature, signature2) // Should produce same signature

	// Verify second signature
	resp, err = ks.Verify(ctx, keystore.VerifyRequest{
		KeyType:   keystore.Ed25519,
		PublicKey: keys.Keys[0].KeyInfo.PublicKey,
		Data:      data,
		Signature: signature2,
	})
	require.NoError(t, err)
	require.True(t, resp.Valid)

	// Test error case: key not found
	_, err = coreKs.Sign(ctx, "invalid-address", data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "key not found")

	// Test WithAllowedKeyNames: create another key and verify it's filtered
	txKey2, err := solanaks.CreateTxKey(ks, "key2")
	require.NoError(t, err)

	coreKsFiltered := solanaks.NewTxKeyCoreKeystore(ks, solanaks.WithAllowedKeyNames([]string{txKey.KeyPath().String()}))
	accountsFiltered, err := coreKsFiltered.Accounts(ctx)
	require.NoError(t, err)
	require.Len(t, accountsFiltered, 1)
	require.Equal(t, txKey.Address().String(), accountsFiltered[0])
	require.NotContains(t, accountsFiltered, txKey2.Address().String())
}
