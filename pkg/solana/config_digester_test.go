package solana

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/require"
)

func TestConfigDigester(t *testing.T) {
	programID, err := solana.PublicKeyFromBase58("Fg6PaFpoGXkYsidMpWTK6W2BeZ7FEfcYkg476zPFsLnS")
	require.NoError(t, err)
	signers, transmitters, err := sortOraclesBySigningKey(
		[]types.OnchainPublicKey{
			[]byte{
				248, 88, 208, 95, 175, 85,
				87, 99, 226, 128, 185, 30,
				29, 234, 255, 71, 70, 64,
				182, 128,
			},
			[]byte{
				31, 57, 135, 52, 16, 217,
				113, 121, 251, 87, 101, 171,
				4, 188, 40, 246, 114, 186,
				246, 155,
			},
			[]byte{
				98, 103, 184, 219, 96, 84,
				91, 30, 31, 59, 145, 231,
				252, 52, 151, 234, 250, 249,
				50, 116,
			},
			[]byte{
				204, 70, 193, 22, 235, 242,
				79, 189, 191, 227, 255, 115,
				200, 178, 163, 243, 239, 100,
				132, 104,
			},
			[]byte{
				224, 66, 21, 88, 253, 234,
				193, 120, 55, 201, 252, 101,
				40, 7, 136, 80, 209, 209,
				80, 97,
			},
			[]byte{
				30, 133, 136, 230, 124, 172,
				171, 217, 37, 145, 66, 203,
				88, 176, 109, 62, 198, 97,
				69, 43,
			},
			[]byte{
				229, 8, 123, 83, 42, 39,
				124, 183, 180, 216, 171, 157,
				163, 239, 18, 59, 178, 135,
				235, 156,
			},
			[]byte{
				170, 140, 34, 63, 43, 86,
				232, 162, 189, 97, 170, 243,
				246, 255, 174, 131, 227, 155,
				201, 173,
			},
			[]byte{
				231, 185, 93, 247, 190, 229,
				234, 238, 123, 196, 117, 127,
				228, 246, 218, 31, 45, 16,
				221, 117,
			},
			[]byte{
				159, 12, 15, 52, 41, 28,
				87, 242, 22, 114, 225, 211,
				179, 248, 155, 91, 31, 110,
				38, 39,
			},
			[]byte{
				42, 245, 120, 190, 71, 62,
				34, 112, 14, 90, 112, 5,
				217, 179, 159, 216, 136, 118,
				171, 126,
			},
			[]byte{
				105, 179, 220, 25, 15, 46,
				178, 173, 225, 168, 59, 158,
				70, 190, 98, 136, 116, 55,
				55, 33,
			},
			[]byte{
				107, 86, 155, 39, 88, 243,
				252, 18, 179, 105, 239, 3,
				152, 225, 231, 71, 155, 86,
				139, 208,
			},
			[]byte{
				182, 211, 22, 22, 49, 96,
				75, 132, 131, 79, 77, 126,
				112, 34, 114, 140, 106, 233,
				192, 246,
			},
			[]byte{
				102, 193, 80, 54, 14, 154,
				158, 180, 98, 176, 203, 113,
				241, 147, 145, 135, 237, 31,
				0, 198,
			},
			[]byte{
				232, 159, 15, 3, 242, 245,
				218, 44, 241, 245, 91, 215,
				23, 179, 84, 195, 224, 53,
				75, 56,
			},
			[]byte{
				187, 104, 212, 250, 52, 149,
				161, 28, 165, 211, 24, 131,
				220, 237, 189, 87, 154, 149,
				15, 55,
			},
		},
		[]types.Account{
			"9bH6g4r5i4MPiCPuCHcWk81gyMrBqjDu1uXrEtbNCM1r",
			"H7MRjkFACSqUG9wY3fLWjtEJLNSXXNebRou848voSAQt",
			"hs34SHfk2o33aa49QyAmqNCqjtS9VFtDejACp8S3kxA",
			"x23iGKycoh4x6CDbVa8AAvgodsQePFsuXRURpZNTeqc",
			"2TVvM7kRu1ZaCQjnYyNgF5qdmcKJLzjNJPRjawk7YU44",
			"8vC4xhK2D2gsdwbwixrQ3VE5QfEccfbPEvSLQxkt1PCo",
			"Gsno5WBauxvJx1xFiqRgQUJCjTJMq94pyGyJhyrsRfM5",
			"BSUkHB26hBtML11GEF2XHnW2Uajp75hYBowJsdoFvz3K",
			"GZJLp5tS4BSCJyCF7rznu4BY3c7qADKjQw8pdv9WhfQ6",
			"ByqAqDd2GNFoVYymFjhvpAZuRbFeN5g4Vd87maez2L7K",
			"3VwXdjNZzHEpnQbrVXCMJuJ4xP2q137xnXUybr7ehh8Z",
			"6z8qKQxERzsLwfdgTx82xQ9Wcd16AyNSCRGMRzPpWR4g",
			"DSsiv4dH3Vkeg2Zf6XWWZp5cdmStTySaoRr9xnR8x7dv",
			"5kJqj5TQM5JC3sTz6ZhpCJQhFk4YwJ3aoHsh6WRYET5P",
			"Gu6TX6GQm1EyjmbN4ZtcaMzZWgXrokiyoznQidUVMQ9X",
			"AvF9iFe79xKYFqAsmC59LJirsa5TdLwYAM84vunuRPea",
			"DdbtwA56jHheniC3EfUHksrnQGE51Ef2hba1g9UyZS6v",
		},
	)
	require.NoError(t, err)
	contract := ContractTracker{
		programAccount: programID,
	}
	config := types.ContractConfig{
		ConfigCount:           1,
		Signers:               signers,
		Transmitters:          transmitters,
		F:                     2,
		OnchainConfig:         []byte{1, 2, 3},
		OffchainConfigVersion: 1,
		OffchainConfig:        []byte{4, 5, 6},
	}
	actualDigest, err := contract.ConfigDigest(config)
	require.NoError(t, err)
	expectedDigest := [32]byte{
		0, 3, 177, 128, 128, 180, 135,
		96, 131, 101, 104, 113, 166, 233,
		115, 112, 175, 249, 227, 96, 54,
		141, 169, 132, 247, 198, 153, 189,
		117, 181, 96, 131,
	}
	require.Equal(t, expectedDigest, [32]byte(actualDigest))
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
