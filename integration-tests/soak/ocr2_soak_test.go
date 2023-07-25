package tests

import (
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
)

func TestSolanaOCRV2SoakTest(t *testing.T) {
	state := common.NewOCRv2State(t, 5, "soak", "localnet")
	if state.Common.Env.WillUseRemoteRunner() {
		// run the remote runner and exit
		err := state.Common.Env.Run()
		require.NoError(t, err)
		return
	}
	state.DeployCluster(utils.ContractsDir)
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewSoakRoundsCheckTimeout, 20000)
}
