package utils

import (
	"crypto/rand"

	"github.com/gagliardetto/solana-go"
)

// NewHash returns a random solana.Hash using SHA-256.
func NewHash() solana.Hash {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return solana.HashFromBytes(b)
}
