package solana

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/require"
)

func TestConfigDigester(t *testing.T) {
	programID, err := solana.PublicKeyFromBase58("My11111111111111111111111111111111111111111")
	require.NoError(t, err)
	digester := OffchainConfigDigester{
		ProgramID: programID,
	}

	// Test ConfigDigester by using a known raw state account + known expected digest
	var state State
	err = bin.NewBorshDecoder(mockRawState).Decode(&state)
	require.NoError(t, err)
	config := configFromState(state)

	actualDigest, err := digester.ConfigDigest(config)
	require.NoError(t, err)

	expectedDigest := "00039ad1eaa13649831b4cc02f5437bf556910037c6833f87249edbeceab2828"
	require.Equal(t, expectedDigest, actualDigest.Hex())
}

// Helpers

type tmpOracleKeys struct {
	signerKey   types.OnchainPublicKey
	transmitter types.Account
}

func sortOraclesBySigningKey(
	signers []types.OnchainPublicKey,
	transmitters []types.Account,
) (
	[]types.OnchainPublicKey,
	[]types.Account,
	error,
) {
	if len(signers) != len(transmitters) {
		return nil, nil, fmt.Errorf(
			"number of signers (%d) and transmitters (%d) is different",
			len(signers), len(transmitters))
	}

	oracles := []tmpOracleKeys{}
	for i := 0; i < len(signers); i++ {
		oracles = append(oracles, tmpOracleKeys{
			signers[i],
			transmitters[i],
		})
	}
	sort.SliceStable(oracles, func(i, j int) bool {
		return bytes.Compare(oracles[i].signerKey, oracles[j].signerKey) < 0
	})
	newSigners := []types.OnchainPublicKey{}
	newTransmitters := []types.Account{}
	for i := 0; i < len(oracles); i++ {
		newSigners = append(newSigners, oracles[i].signerKey)
		newTransmitters = append(newTransmitters, oracles[i].transmitter)
	}

	return newSigners, newTransmitters, nil
}
