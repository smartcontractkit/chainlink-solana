package utils

import (
	"crypto/rand"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

// NewSolanaHash returns a random solana.Hash using SHA-256.
func NewSolanaHash() solana.Hash {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return solana.HashFromBytes(b)
}

func NewHash() types.Hash {
	return types.Hash{
		Hash: NewSolanaHash(),
	}
}
