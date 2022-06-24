package monitoring

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/stretchr/testify/require"
)

func TestIngestor(t *testing.T) {
	t.Run("DecoderAndMapper", func(t *testing.T) {
		solanaConfig := generateChainConfig()
		feedConfig := generateStaticFeedConfig()

		for _, testCase := range []struct {
			testName       string
			input          interface{}
			decoder        Decoder
			mapper         Mapper
			expectedOutput map[string]interface{}
		}{
			{
				"LogResult",
				&ws.LogResult{
					Context: struct {
						Slot uint64
					}{
						Slot: 0x844e6ed,
					}, Value: struct {
						Signature solana.Signature "json:\"signature\""
						Err       interface{}      "json:\"err\""
						Logs      []string         "json:\"logs\""
					}{
						Signature: solana.Signature{0xd4, 0x8a, 0x3e, 0x48, 0xde, 0x4, 0xd5, 0x5c, 0xef, 0xab, 0xd3, 0xf5, 0x7d, 0xca, 0x5b, 0x1a, 0x46, 0x5c, 0x7a, 0xdf, 0xb5, 0x84, 0x89, 0xfb, 0x12, 0xa0, 0x4b, 0xb0, 0x1c, 0x94, 0xd7, 0x86, 0x15, 0xd0, 0x2a, 0x14, 0x6d, 0xad, 0x92, 0xdf, 0xa1, 0xa7, 0x5f, 0xcb, 0x77, 0x23, 0xd9, 0x75, 0x37, 0x99, 0x3b, 0x33, 0xa5, 0xf9, 0x14, 0x9, 0xb9, 0x87, 0xa9, 0x7c, 0x6b, 0xc0, 0xc0, 0xd},
						Err:       interface{}(nil),
						Logs: []string{
							"Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ invoke [1]",
							"Program data: gjbLTR5rT6hqABgAAANEdd2a8hexDBEvBE3+tcab4BGo3GveMERFF3rmDkyABGahGQAAAAAAAAAAAAAADwm5tGIQDgMFDAAKCQEHCw0GDwgEAgAAACLV/UIBAAAA1mkAAAAAAAA=", "Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny invoke [2]",
							"Program log: Instruction: Submit",
							"Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny consumed 3828 of 1211486 compute units",
							"Program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny success",
							"Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ consumed 192912 of 1400000 compute units",
							"Program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ success",
						},
					},
				},
				LogResultDecode,
				LogMapper,
				map[string]interface{}{
					"err": "",
					"events": []interface{}{map[string]interface{}{
						"link.chain.ocr2.ocr2_event_new_transmission": map[string]interface{}{
							"answer":                 []uint8{0x19, 0xa1, 0x66, 0x4, 0x80},
							"config_digest":          []uint8{0x0, 0x3, 0x44, 0x75, 0xdd, 0x9a, 0xf2, 0x17, 0xb1, 0xc, 0x11, 0x2f, 0x4, 0x4d, 0xfe, 0xb5, 0xc6, 0x9b, 0xe0, 0x11, 0xa8, 0xdc, 0x6b, 0xde, 0x30, 0x44, 0x45, 0x17, 0x7a, 0xe6, 0xe, 0x4c},
							"juels_per_lamport":      []uint8{0x0, 0x0, 0x0, 0x1, 0x42, 0xfd, 0xd5, 0x22},
							"observations_timestamp": int64(1656011017),
							"observer_count":         int32(16),
							"observers":              []int{14, 3, 5, 12, 0, 10, 9, 1, 7, 11, 13, 6, 15, 8, 4, 2, 0, 0, 0},
							"reimbursement_gjuels":   []uint8{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x69, 0xd6},
							"round_id":               int64(1572970),
							"transmitter":            int32(15),
						},
					}},
					"program_public_key": []uint8{0x9, 0x27, 0x94, 0x40, 0x91, 0x2c, 0x68, 0x97, 0xfc, 0x6d, 0x25, 0x62, 0xca, 0x65, 0x78, 0xf5, 0x9, 0x27, 0xf6, 0x82, 0x67, 0x70, 0x2e, 0x3f, 0x59, 0xfb, 0x33, 0xc5, 0x68, 0x9, 0xa1, 0x75},
					"signature":          []uint8{0xd4, 0x8a, 0x3e, 0x48, 0xde, 0x4, 0xd5, 0x5c, 0xef, 0xab, 0xd3, 0xf5, 0x7d, 0xca, 0x5b, 0x1a, 0x46, 0x5c, 0x7a, 0xdf, 0xb5, 0x84, 0x89, 0xfb, 0x12, 0xa0, 0x4b, 0xb0, 0x1c, 0x94, 0xd7, 0x86, 0x15, 0xd0, 0x2a, 0x14, 0x6d, 0xad, 0x92, 0xdf, 0xa1, 0xa7, 0x5f, 0xcb, 0x77, 0x23, 0xd9, 0x75, 0x37, 0x99, 0x3b, 0x33, 0xa5, 0xf9, 0x14, 0x9, 0xb9, 0x87, 0xa9, 0x7c, 0x6b, 0xc0, 0xc0, 0xd},
					"slot":               []uint8{0x0, 0x0, 0x0, 0x0, 0x8, 0x44, 0xe6, 0xed},
				},
			},
		} {
			t.Run(testCase.testName, func(t *testing.T) {
				decoded, err := testCase.decoder(testCase.input, solanaConfig, feedConfig)
				require.NoError(t, err)
				actual, err := testCase.mapper(decoded, solanaConfig, feedConfig)
				require.NoError(t, err)
				require.Equal(t, testCase.expectedOutput, actual)
			})
		}
	})
}

// Helpers

func generateStaticFeedConfig() SolanaFeedConfig {
	feedConfig := generateFeedConfig()
	feedConfig.ContractAddressBase58 = "cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ"
	feedConfig.TransmissionsAccountBase58 = "CGmWwBNsTRDENT5gmVZzRu38GnNnMm1K5C3sFiUUyYQX"
	feedConfig.StateAccountBase58 = "2oyA8ZLwuWeAR5ANyDsiEGueUyDC8jFGFLSixSzT9KtV"
	feedConfig.ContractAddress = solana.MustPublicKeyFromBase58(feedConfig.ContractAddressBase58)
	feedConfig.TransmissionsAccount = solana.MustPublicKeyFromBase58(feedConfig.TransmissionsAccountBase58)
	feedConfig.StateAccount = solana.MustPublicKeyFromBase58(feedConfig.StateAccountBase58)
	return feedConfig
}
