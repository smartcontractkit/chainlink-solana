package solana

import (
	"testing"

	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockPrivateKey = []byte("a 32 length private key for test")

func TestOnchainKeyring(t *testing.T) {
	kr, err := NewOnchainKeyring(mockPrivateKey)
	require.NoError(t, err)

	assert.Equal(t, 64, len(kr.PublicKey())) // compressed = false, skip first byte (0x04)

	// sign empty message
	sig, err := kr.Sign(types.ReportContext{}, types.Report{})
	require.NoError(t, err)
	assert.Equal(t, kr.MaxSignatureLength(), len(sig)) // check length

	// verify that the signed message matches the public key
	ok := kr.Verify(kr.PublicKey(), types.ReportContext{}, types.Report{}, sig)
	assert.True(t, ok) // check verify
}

func TestOnchainKeyring_Fail(t *testing.T) {
	_, err := NewOnchainKeyring(mockPrivateKey[:31])
	assert.Error(t, err)
}
