package logpoller

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func newRandomPublicKey(t *testing.T) PublicKey {
	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	return PublicKey(pubKey)
}

func newRandomEventSignature(t *testing.T) EventSignature {
	pubKey := newRandomPublicKey(t)
	return EventSignature(pubKey[:8])
}
