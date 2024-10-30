package chainwriter_test

// import (
// 	"context"
// 	"testing"

// 	"github.com/gagliardetto/solana-go"
// 	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
// 	"github.com/test-go/testify/assert"
// 	"github.com/test-go/testify/require"
// )

// type TestStruct struct {
// 	Messages []Message
// }

// type Message struct {
// 	TokenAmounts []TokenAmount
// }

// type TokenAmount struct {
// 	SourceTokenAddress []byte
// 	DestTokenAddress   []byte
// }

// func TestHelpersTestGetAddresses(t *testing.T) {
// 	ctx := context.TODO()

// 	chainWriterConfig := chainwriter.ChainWriterConfig{}
// 	service := chainwriter.NewChainWriterService(chainWriterConfig)

// 	t.Run("success with AccountConstant", func(t *testing.T) {
// 		accounts := []chainwriter.Lookup{
// 			chainwriter.AccountConstant{
// 				Name:       "test-account",
// 				Address:    "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M",
// 				IsSigner:   true,
// 				IsWritable: false,
// 			},
// 		}

// 		// Call GetAddresses with the constant account
// 		addresses, err := service.GetAddresses(ctx, nil, accounts, nil, "test-debug-id")
// 		require.NoError(t, err)
// 		require.Len(t, addresses, 1)
// 		require.Equal(t, addresses[0].PublicKey.String(), "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6M")
// 		require.True(t, addresses[0].IsSigner)
// 		require.False(t, addresses[0].IsWritable)
// 	})

// 	t.Run("success with AccountLookup", func(t *testing.T) {
// 		accounts := []chainwriter.Lookup{
// 			chainwriter.AccountLookup{
// 				Name:       "test-account",
// 				Location:   "Messages.TokenAmounts.SourceTokenAddress",
// 				IsSigner:   true,
// 				IsWritable: false,
// 			},
// 			chainwriter.AccountLookup{
// 				Name:       "test-account",
// 				Location:   "Messages.TokenAmounts.DestTokenAddress",
// 				IsSigner:   true,
// 				IsWritable: false,
// 			},
// 		}

// 		// Create a test struct with the expected address
// 		addresses := make([][]byte, 8)
// 		for i := 0; i < 8; i++ {
// 			privKey, err := solana.NewRandomPrivateKey()
// 			require.NoError(t, err)
// 			addresses[i] = privKey.PublicKey().Bytes()
// 		}

// 		exampleDecoded := TestStruct{
// 			Messages: []Message{
// 				{
// 					TokenAmounts: []TokenAmount{
// 						{addresses[0], addresses[1]},
// 						{addresses[2], addresses[3]},
// 					},
// 				},
// 				{
// 					TokenAmounts: []TokenAmount{
// 						{addresses[4], addresses[5]},
// 						{addresses[6], addresses[7]},
// 					},
// 				},
// 			},
// 		}
// 		// Call GetAddresses with the lookup account
// 		derivedAddresses, err := service.GetAddresses(ctx, exampleDecoded, accounts, nil, "test-debug-id")

// 		// Create a map of the expected addresses for fast lookup
// 		expectedAddresses := make(map[string]bool)
// 		for _, addr := range addresses {
// 			expectedAddresses[string(addr)] = true
// 		}

// 		// Verify that each derived address matches an expected address
// 		for _, derivedAddr := range derivedAddresses {
// 			derivedBytes := derivedAddr.PublicKey.Bytes()
// 			assert.True(t, expectedAddresses[string(derivedBytes)], "Address not found in expected list")
// 		}

// 		require.NoError(t, err)
// 		require.Len(t, derivedAddresses, 8)
// 	})
// }
