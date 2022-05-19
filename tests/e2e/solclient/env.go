package solclient

import (
	"github.com/smartcontractkit/chainlink-testing-framework/config"
	"github.com/smartcontractkit/helmenv/environment"
)

// NewChainlinkSolOCRv2 returns a cluster config with Solana test validator
func NewChainlinkSolOCRv2(nodes int, stateful bool) *environment.Config {
	config.ChainlinkVals()
	env := &environment.Config{
		NamespacePrefix: "chainlink-sol",
		Charts: environment.Charts{
			"solana-validator": {
				Index: 1,
			},
			"mockserver-config": {
				Index: 2,
			},
			"mockserver": {
				Index: 3,
			},
			"chainlink": {
				Index: 4,
				Values: map[string]interface{}{
					"replicas": nodes,
					"chainlink": map[string]interface{}{
						"image": map[string]interface{}{
							"image":   config.ProjectFrameworkSettings.ChainlinkImage,
							"version": config.ProjectFrameworkSettings.ChainlinkVersion,
						},
					},
					"env": map[string]interface{}{
						"SOLANA_ENABLED":              "true",
						"EVM_ENABLED":                 "false",
						"EVM_RPC_ENABLED":             "false",
						"CHAINLINK_DEV":               "false",
						"FEATURE_OFFCHAIN_REPORTING2": "true",
						"P2P_NETWORKING_STACK":        "V2",
						"P2PV2_LISTEN_ADDRESSES":      "0.0.0.0:6690",
						"P2PV2_DELTA_DIAL":            "5s",
						"P2PV2_DELTA_RECONCILE":       "5s",
						"p2p_listen_port":             "0",
					},
				},
			},
		},
	}
	if stateful {
		env.Charts["chainlink"].Values["db"] = map[string]interface{}{
			"stateful": true,
			"capacity": "2Gi",
		}
	}
	return env
}
