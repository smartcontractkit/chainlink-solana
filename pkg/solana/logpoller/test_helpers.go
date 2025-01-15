package logpoller

import (
	"math/rand"
	"testing"
	"time"

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

func newRandomLog(t *testing.T, filterID int64, chainID string, eventName string) Log {
	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	data := []byte("solana is fun")
	signature, err := privateKey.Sign(data)
	require.NoError(t, err)
	return Log{
		FilterID:       filterID,
		ChainID:        chainID,
		LogIndex:       rand.Int63n(1000),
		BlockHash:      Hash(pubKey),
		BlockNumber:    rand.Int63n(1000000),
		BlockTimestamp: time.Unix(1731590113, 0).UTC(),
		Address:        PublicKey(pubKey),
		EventSig:       Discriminator("event", eventName),
		SubkeyValues:   []IndexedValue{{3, 2, 1}, {1}, {1, 2}, pubKey.Bytes()},
		TxHash:         Signature(signature),
		Data:           data,
		SequenceNum:    rand.Int63n(500),
	}
}
