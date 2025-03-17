package config

import (
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/stretchr/testify/require"
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
		require.Error(t, node.ValidateConfig())
	})
	t.Run("Empty Node name", func(t *testing.T) {
		t.Parallel()
		nodeName := ""
		url := config.MustParseURL("http://url.com")
		node := Node{
			Name: &nodeName,
			URL:  url,
		}
		require.Error(t, node.ValidateConfig())
	})
	t.Run("Null Node URL", func(t *testing.T) {
		t.Parallel()
		nodeName := "node"
		node := Node{
			Name: &nodeName,
			URL:  nil,
		}
		require.Error(t, node.ValidateConfig())
	})
	t.Run("Empty Node URL", func(t *testing.T) {
		t.Parallel()
		nodeName := "node"
		url := config.MustParseURL("")
		node := Node{
			Name: &nodeName,
			URL:  url,
		}
		require.Error(t, node.ValidateConfig())
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
