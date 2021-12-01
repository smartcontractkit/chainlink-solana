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

func TestOnchainKeyring_reportHash(t *testing.T) {
	var mockDigest = [32]byte{
		0, 3, 94, 221, 213, 66, 228, 80, 239, 231, 7, 96,
		83, 156, 95, 165, 199, 168, 222, 107, 47, 238, 157, 46,
		65, 205, 71, 121, 195, 138, 77, 137,
	}
	var mockReportCtx = types.ReportContext{
		ReportTimestamp: types.ReportTimestamp{
			ConfigDigest: mockDigest,
			Epoch:        1,
			Round:        1,
		},
		ExtraHash: [32]byte{},
	}

	var mockReport = types.Report{
		97, 91, 43, 83, // observations_timestamp
		0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // observers
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 210, // median
		0, 0, 0, 0, 0, 0, 0, 0, 13, 224, 182, 179, 167, 100, 0, 0, // juels per sol (1 with 18 decimal places)
	}

	var mockHash = []byte{
		124, 158, 204, 40, 181, 54, 124,
		38, 196, 146, 13, 14, 178, 47,
		254, 150, 111, 21, 42, 181, 191,
		132, 111, 236, 216, 151, 233, 110,
		86, 216, 154, 169,
	}

	h, err := reportHash(mockReportCtx, mockReport)
	assert.NoError(t, err)
	assert.Equal(t, mockHash, h)
}
