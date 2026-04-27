package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	t.Run("Null Node name", func(t *testing.T) {
		t.Parallel()
		url := config.MustParseURL("http://url.com")
		node := Node{
			Name: nil,
			URL:  url,
		}
		require.ErrorIs(t, node.ValidateConfig(), config.ErrMissing{Name: "Name", Msg: "required for all nodes"})
	})
	t.Run("Empty Node name", func(t *testing.T) {
		t.Parallel()
		nodeName := ""
		url := config.MustParseURL("http://url.com")
		node := Node{
			Name: &nodeName,
			URL:  url,
		}
		require.ErrorIs(t, node.ValidateConfig(), config.ErrEmpty{Name: "Name", Msg: "required for all nodes"})
	})
	t.Run("Null Node URL", func(t *testing.T) {
		t.Parallel()
		nodeName := "node"
		node := Node{
			Name: &nodeName,
			URL:  nil,
		}
		require.ErrorIs(t, node.ValidateConfig(), config.ErrMissing{Name: "URL", Msg: "required for all nodes"})
	})
	t.Run("Empty Node URL", func(t *testing.T) {
		t.Parallel()
		nodeName := "node"
		url := config.MustParseURL("")
		node := Node{
			Name: &nodeName,
			URL:  url,
		}
		require.ErrorIs(t, node.ValidateConfig(), config.ErrEmpty{Name: "URL", Msg: "required for all nodes"})
	})
	t.Run("Valid config", func(t *testing.T) {
		t.Parallel()
		nodeName := "node"
		url := config.MustParseURL("http://url.com")
		node := Node{
			Name: &nodeName,
			URL:  url,
		}
		require.NoError(t, node.ValidateConfig())
	})
}

func TestWorkflowConfigSetFrom(t *testing.T) {
	var w WorkflowConfig
	timeout := config.MustNewDuration(10 * time.Second)
	state := commontypes.Finalized
	gasLimitDefault := uint64(400)
	local := true
	pollPeriod := config.MustNewDuration(1 * time.Second)
	other := WorkflowConfig{AcceptanceTimeout: timeout, PollPeriod: pollPeriod, Local: &local, GasLimitDefault: &gasLimitDefault, TxAcceptanceState: &state}
	w.SetFrom(&other)
	require.Equal(t, timeout, w.AcceptanceTimeout)
	require.Equal(t, pollPeriod, w.PollPeriod)
	require.Equal(t, local, *w.Local)
	require.Equal(t, gasLimitDefault, *w.GasLimitDefault)
	require.Equal(t, state, *w.TxAcceptanceState)

}

func TestWorkflowConfigIsEnabled(t *testing.T) {
	t.Run("nil fields", func(t *testing.T) {
		require.False(t, new(WorkflowConfig).IsEnabled())
	})
	t.Run("only acceptance timeout", func(t *testing.T) {
		cfg := WorkflowConfig{
			AcceptanceTimeout: config.MustNewDuration(time.Second),
		}
		require.False(t, cfg.IsEnabled())
	})
	t.Run("only poll period", func(t *testing.T) {
		cfg := WorkflowConfig{
			PollPeriod: config.MustNewDuration(time.Second),
		}
		require.False(t, cfg.IsEnabled())
	})
	t.Run("both set with positive duration", func(t *testing.T) {
		cfg := WorkflowConfig{
			AcceptanceTimeout: config.MustNewDuration(45 * time.Second),
			PollPeriod:        config.MustNewDuration(3 * time.Second),
		}
		require.True(t, cfg.IsEnabled())
	})
	t.Run("both set but zero duration", func(t *testing.T) {
		cfg := WorkflowConfig{
			AcceptanceTimeout: config.MustNewDuration(0),
			PollPeriod:        config.MustNewDuration(0),
		}
		require.False(t, cfg.IsEnabled())
	})
}
