package smoke

import (
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	state := common.NewOCRv2State(t, 1, "smoke")
	state.DeployCluster(utils.ContractsDir)
	if state.Common.Env.WillUseRemoteRunner() {
		return
	}
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
}
