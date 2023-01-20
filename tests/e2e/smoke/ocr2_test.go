package smoke

import (
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink/integration-tests/actions"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	var state = common.NewOCRv2State(t, 1, 5)
	state.DeployCluster(5, false, utils.ContractsDir, 30*time.Minute)
	if state.Env.WillUseRemoteRunner() {
		return
	}
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
	err := actions.TeardownSuite(state.T, state.Env, "logs", state.ChainlinkNodes, nil, nil)
	require.NoError(t, err)
}
