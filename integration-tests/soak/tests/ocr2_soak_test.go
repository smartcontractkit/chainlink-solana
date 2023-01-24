package tests

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
	"github.com/smartcontractkit/chainlink/integration-tests/actions"
)

func TestSolanaOCRV2SoakTest(t *testing.T) {
	var state = common.NewOCRv2State(t, 5)
	state.DeployCluster(utils.ContractsDir)
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), state.Common.TTL, 10000000)
	err := actions.TeardownSuite(state.T, state.Common.Env, "logs", state.ChainlinkNodes, nil, nil)
	require.NoError(t, err)
}
