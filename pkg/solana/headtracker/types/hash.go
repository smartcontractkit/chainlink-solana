package headtracker

import (
	"github.com/gagliardetto/solana-go"
)

type Hash struct {
	solana.Hash
}

func (h Hash) Bytes() []byte {
	return h.Hash[:]
}
