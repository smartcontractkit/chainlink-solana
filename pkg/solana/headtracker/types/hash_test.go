package types

import (
	"bytes"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestHash_Bytes(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		// Create a solana.Hash with 32 bytes.
		expectedBytes := []byte("abcdefghabcdefghabcdefghabcdefgh")
		var solanaHash solana.Hash
		copy(solanaHash[:], expectedBytes)

		// Create a Hash instance with the solana.Hash we just created.
		testHash := Hash{solanaHash}
		actualBytes := testHash.Bytes()

		// Check that the bytes returned by the method match the bytes we put into the solana.Hash.
		if !bytes.Equal(actualBytes, expectedBytes) {
			t.Errorf("Bytes() returned %v, want %v", actualBytes, expectedBytes)
		}
	})
}
