package smoke

import (
	"testing"

	"github.com/rs/zerolog/log"

	tc "github.com/smartcontractkit/chainlink-solana/integration-tests/testconfig"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

func TestSolanaOCRV2UpgradeSmoke(t *testing.T) {
	name := "plugins-program-upgrade"
	env := map[string]string{
		"CL_MEDIAN_CMD": "chainlink-feeds",
		"CL_SOLANA_CMD": "chainlink-solana",
	}
	config, err := tc.GetConfig("Smoke", tc.OCR2)
	if err != nil {
		t.Fatal(err)
	}
	s, sg := startOCR2DataFeedsSmokeTest(t, name, env, config, "previous")
	// validate cluster is functioning
	validateRounds(t, name, sg, *config.OCR2.NumberOfRounds)

	// make it very obvious with logging for redeploying contracts
	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|           REDEPLOYING CONTRACTS           |")
	log.Info().Msg("---------------------------------------------")
	s.UpgradeContracts(utils.ContractsDir, "")
	log.Info().Msg("---------------------------------------------")
	log.Info().Msg("|                                           |")
	log.Info().Msg("---------------------------------------------")

	// validate cluster is still functioning after program upgrade
	validateRounds(t, name, sg, *config.OCR2.NumberOfRounds)
}
