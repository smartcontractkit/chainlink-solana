package tests

import (
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
)

func TestSolanaOCRV2SoakTest(t *testing.T) {
	state := common.NewOCRv2State(t, 5, "soak")
	state.DeployCluster(utils.ContractsDir)
	if state.Common.Env.WillUseRemoteRunner() {
		return
	}
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), state.Common.TTL, 10000000)
}
