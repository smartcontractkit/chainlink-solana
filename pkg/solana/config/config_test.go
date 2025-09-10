package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
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

func TestWorkflowConfigSetEnabled(t *testing.T) {
	var cfg WorkflowConfig
	require.False(t, cfg.IsEnabled())
}
