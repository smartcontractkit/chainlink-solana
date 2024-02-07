package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tc "github.com/smartcontractkit/chainlink/integration-tests/testconfig"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

func TestSolanaOCRV2SoakTest(t *testing.T) {
	config, err := tc.GetConfig("Smoke", tc.OCR2)
	if err != nil {
		t.Fatal(err)
	}
	state, err := common.NewOCRv2State(t, 5, "soak", "devnet", true, &config)
	require.NoError(t, err, "Could not setup the ocrv2 state")
	if state.Common.Env.WillUseRemoteRunner() {
		// run the remote runner and exit
		err := state.Common.Env.Run()
		require.NoError(t, err)
		return
	}
	state.DeployCluster(utils.ContractsDir)
	state.ValidateRoundsAfter(time.Now(), common.NewSoakRoundsCheckTimeout, 20000)
}
