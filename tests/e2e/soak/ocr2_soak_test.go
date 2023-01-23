package smoke

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
	"github.com/smartcontractkit/chainlink/integration-tests/actions"
)

func TestSolanaOCRV2SoakTest(t *testing.T) {
	var state = common.NewOCRv2State(t, 30, 5)
	state.DeployCluster(5, false, utils.ContractsDir)
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewSoakRoundsCheckTimeout, 200)
	err := actions.TeardownSuite(state.T, state.Env, "logs", state.ChainlinkNodes, nil, nil)
	require.NoError(t, err)
}
