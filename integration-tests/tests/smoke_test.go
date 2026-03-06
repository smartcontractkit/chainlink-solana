package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products/solana"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
)

const defaultEnvOutPath = "../devenv/env-out.toml"

// TestSolanaOCRV2Smoke reads the environment output and validates OCR rounds.
// Whether the environment was started with embedded or plugins mode is
// determined entirely by the environment setup phase (CL_MEDIAN_CMD /
// CL_SOLANA_CMD env vars passed to containers at startup). This test has
// zero knowledge of the plugin mode -- it just reads env-out.toml.
func TestSolanaOCRV2Smoke(t *testing.T) {
	pdConfig, err := products.LoadOutput[solana.Configurator](defaultEnvOutPath)
	require.NoError(t, err, "Failed to load product config from env-out.toml")
	require.NotEmpty(t, pdConfig.Config, "No OCR2 Solana config found in env-out.toml")

	cfg := pdConfig.Config[0]
	sg, err := gauntlet.NewSolanaGauntlet(cfg.GauntletPath)
	require.NoError(t, err, "Failed to reconstruct gauntlet from saved path")
	sg.G.Network = cfg.GauntletNetwork
	sg.G.Command = "gauntlet-nobuild"

	validateRounds(t, cfg.OcrAddress, sg, cfg.NumberOfRounds)
}

func validateRounds(t *testing.T, ocrAddress string, sg *gauntlet.SolanaGauntlet, rounds int) {
	t.Helper()
	stuck := 0
	successfulRounds := 0
	prevRound := gauntlet.Transmission{RoundID: 0}

	for successfulRounds < rounds {
		time.Sleep(6 * time.Second)
		require.Less(t, stuck, 10, "Rounds have been stuck for more than 10 iterations")

		log.Info().Str("Transmission", ocrAddress).Msg("Inspecting transmissions")
		transmissions, err := sg.FetchTransmissions(ocrAddress)
		require.NoError(t, err)

		if len(transmissions) <= 1 {
			log.Info().Str("Contract", ocrAddress).Msg("No transmissions yet")
			stuck++
			continue
		}

		currentRound := getLatestRound(transmissions)
		if prevRound.RoundID == 0 {
			prevRound = currentRound
		}
		if currentRound.RoundID <= prevRound.RoundID {
			log.Info().Str("Transmission", ocrAddress).Msg("No new transmissions")
			stuck++
			continue
		}

		log.Info().
			Str("Contract", ocrAddress).
			Interface("Answer", currentRound.Answer).
			Int64("RoundID", currentRound.RoundID).
			Msg("New answer found")
		require.Equal(t, int64(5), currentRound.Answer, fmt.Sprintf("Actual: %d, Expected: 5", currentRound.Answer))
		require.Less(t, prevRound.RoundID, currentRound.RoundID)

		prevRound = currentRound
		successfulRounds++
		stuck = 0
	}
}

func getLatestRound(transmissions []gauntlet.Transmission) gauntlet.Transmission {
	highest := transmissions[0]
	for _, t := range transmissions[1:] {
		if t.RoundID > highest.RoundID {
			highest = t
		}
	}
	return highest
}
