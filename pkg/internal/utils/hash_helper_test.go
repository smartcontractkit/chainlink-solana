package utils_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestNewHash(t *testing.T) {
	t.Parallel()

	h1 := utils.NewHash()
	h2 := utils.NewHash()
	// Check that the two hashes are not the same.
	assert.NotEqual(t, h1, h2)

	// Check that neither hash is equal to a zero hash.
	zeroHash := solana.Hash{}
	assert.NotEqual(t, h1, zeroHash)
	assert.NotEqual(t, h2, zeroHash)
}
