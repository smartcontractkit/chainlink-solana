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
	err = bin.NewBorshDecoder(mockState.Raw).Decode(&state)
	require.NoError(t, err)
	config, err := configFromState(state)
	require.NoError(t, err)

	actualDigest, err := digester.ConfigDigest(config)
	require.NoError(t, err)

	expectedDigest := mockState.ConfigDigestHex
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

// func TestConfigDigester_DeployedContract(t *testing.T) {
// 	tracker := ContractTracker{
// 		StateID:         solana.MustPublicKeyFromBase58("33Y9gByW9aFEsLVruiu2QVBnGuxDhY218Z31sCBiRQ5D"),
// 		TransmissionsID: solana.MustPublicKeyFromBase58("7CgAwVHe7SaTCqwUpMT8QtWc4wSkaVnoFXLaF5fa8qHr"),
// 		client:          NewClient(rpc.LocalNet_RPC, &ws.Client{}),
// 		lggr:            logger.NullLogger,
// 		requestGroup:    &singleflight.Group{},
// 	}
//
// 	digester := OffchainConfigDigester{
// 		ProgramID: solana.MustPublicKeyFromBase58("CF6b2XF6BZw65aznGzXwzF5A8iGhDBoeNYQiXyH4MWdQ"),
// 	}
//
// 	cfg, err := tracker.LatestConfig(context.TODO(), 0)
// 	require.NoError(t, err)
//
// 	digest, err := digester.ConfigDigest(cfg)
// 	require.NoError(t, err)
// 	assert.Equal(t, cfg.ConfigDigest, digest)
// }
