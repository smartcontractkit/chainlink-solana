package solana

import (
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func TestConfigDigester(t *testing.T) {
	programID, err := solana.PublicKeyFromBase58("CF13pnKGJ1WJZeEgVAtFdUi4MMndXm9hneiHs8azUaZt")
	require.NoError(t, err)
	stateID, err := solana.PublicKeyFromBase58("CWhSnpBq3Vkdzfz6ceKBgfdnGegy8XSEXAknVrorFcyW")
	require.NoError(t, err)
	digester := OffchainConfigDigester{
		ProgramID: programID,
		StateID:   stateID,
	}

	// Test ConfigDigester by using a known raw state account + known expected digest
	var state State
	err = bin.NewBorshDecoder(mockState.Raw).Decode(&state)
	require.NoError(t, err)
	config, err := ConfigFromState(state)
	require.NoError(t, err)

	actualDigest, err := digester.ConfigDigest(config)
	require.NoError(t, err)

	expectedDigest := mockState.ConfigDigestHex
	require.Equal(t, expectedDigest, actualDigest.Hex())
}
