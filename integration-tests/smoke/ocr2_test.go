package smoke

import (
	"github.com/smartcontractkit/chainlink/integration-tests/actions"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	var state = common.NewOCRv2State(t, 1)
	state.DeployCluster(utils.ContractsDir)
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
	err := actions.TeardownSuite(state.T, state.Common.Env, "logs", state.ChainlinkNodes, nil, nil)
	require.NoError(t, err)
}
