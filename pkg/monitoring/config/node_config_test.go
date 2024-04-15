package config

import (
	"testing"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type invalidNode struct{}

func (c invalidNode) GetName() string {
	return "INVALID-name"
}

func (c invalidNode) GetAccount() types.Account {
	return "INVALID-account"
}

func TestMakeSolanaNodeConfigs(t *testing.T) {
	validCfg := SolanaNodeConfig{
		ID:          "VALID-name",
		NodeAddress: []string{"VALID-account"},
	}

	params := []struct {
		name    string
		inputs  []commonMonitoring.NodeConfig
		outputs []SolanaNodeConfig
		errStr  string
	}{
		{
			name:    "happy-path",
			inputs:  []commonMonitoring.NodeConfig{validCfg, validCfg},
			outputs: []SolanaNodeConfig{validCfg, validCfg},
		},
		{
			name:   "invalid-0",
			inputs: []commonMonitoring.NodeConfig{invalidNode{}, validCfg},
			errStr: "expected NodeConfig to be of type config.SolanaFeedConfig",
		},
		{
			name:   "invalid-1",
			inputs: []commonMonitoring.NodeConfig{validCfg, invalidNode{}},
			errStr: "expected NodeConfig to be of type config.SolanaFeedConfig",
		},
		{
			name: "empty",
		},
		{
			name:   "nil",
			inputs: nil,
		},
		{
			name:   "nil-config",
			inputs: []commonMonitoring.NodeConfig{nil},
			errStr: "node config is nil",
		},
	}

	for i := range params {
		p := params[i]
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()

			out, err := MakeSolanaNodeConfigs(p.inputs)
			if p.errStr != "" {
				require.ErrorContains(t, err, p.errStr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, p.outputs, out)
		})
	}
}
