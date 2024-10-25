package chainwriter_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/stretchr/testify/assert"
)

type TestStruct struct {
	Messages []Message
}

type Message struct {
	TokenAmounts []TokenAmount
}

type TokenAmount struct {
	SourceTokenAddress []byte
	DestTokenAddress   []byte
}

func TestHelperLookupFunction(t *testing.T) {
	addresses := make([][]byte, 8)
	for i := 0; i < 8; i++ {
		privKey, err := solana.NewRandomPrivateKey()
		assert.NoError(t, err)
		addresses[i] = privKey.PublicKey().Bytes()
	}

	exampleDecoded := TestStruct{
		Messages: []Message{
			{
				TokenAmounts: []TokenAmount{
					{addresses[0], addresses[1]},
					{addresses[2], addresses[3]},
				},
			},
			{
				TokenAmounts: []TokenAmount{
					{addresses[4], addresses[5]},
					{addresses[6], addresses[7]},
				},
			},
		},
	}

	addressLocations := []string{
		"Messages.TokenAmounts.SourceTokenAddress",
		"Messages.TokenAmounts.DestTokenAddress",
	}

	derivedAddresses, err := chainwriter.GetAddressesFromDecodedData(exampleDecoded, addressLocations)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(derivedAddresses))

	// Create a map of the expected addresses for fast lookup
	expectedAddresses := make(map[string]bool)
	for _, addr := range addresses {
		expectedAddresses[string(addr)] = true
	}

	// Verify that each derived address matches an expected address
	for _, derivedAddr := range derivedAddresses {
		derivedBytes := derivedAddr.PublicKey.Bytes()
		assert.True(t, expectedAddresses[string(derivedBytes)], "Address not found in expected list")
	}
}
