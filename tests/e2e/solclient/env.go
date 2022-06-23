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
				Values: map[string]interface{}{
					"nodeSelector": map[string]interface{}{"node-role": "foundations"},
					"tolerations": []map[string]interface{}{{
						"key":      "node-role",
						"operator": "Equal",
						"value":    "foundations",
						"effect":   "NoSchedule",
					}},
				},
			},
			"mockserver-config": {
				Index: 2,
				Values: map[string]interface{}{
					"nodeSelector": map[string]interface{}{"node-role": "foundations"},
					"tolerations": []map[string]interface{}{{
						"key":      "node-role",
						"operator": "Equal",
						"value":    "foundations",
						"effect":   "NoSchedule",
					}},
				},
			},
			"mockserver": {
				Index: 3,
				Values: map[string]interface{}{
					"nodeSelector": map[string]interface{}{"node-role": "foundations"},
					"tolerations": []map[string]interface{}{{
						"key":      "node-role",
						"operator": "Equal",
						"value":    "foundations",
						"effect":   "NoSchedule",
					}},
				},
			},
			"chainlink": {
				Index: 4,
				Values: map[string]interface{}{
					"replicas": nodes,
					"chainlink": map[string]interface{}{
						"image": map[string]interface{}{
							"image":   config.ProjectConfig.FrameworkConfig.ChainlinkImage,
							"version": config.ProjectConfig.FrameworkConfig.ChainlinkVersion,
						},
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"cpu":    "1000m",
								"memory": "2000Mi",
							},
							"limits": map[string]interface{}{
								"cpu":    "1000m",
								"memory": "2000Mi",
							},
						},
					},
					"env": map[string]interface{}{
						"SOLANA_ENABLED":              "true",
						"EVM_ENABLED":                 "false",
						"EVM_RPC_ENABLED":             "false",
						"CHAINLINK_DEV":               "false",
						"FEATURE_OFFCHAIN_REPORTING2": "true",
						"feature_offchain_reporting":  "false",
						"P2P_NETWORKING_STACK":        "V2",
						"P2PV2_LISTEN_ADDRESSES":      "0.0.0.0:6690",
						"P2PV2_DELTA_DIAL":            "5s",
						"P2PV2_DELTA_RECONCILE":       "5s",
						"p2p_listen_port":             "0",
					},
					"nodeSelector": map[string]interface{}{"node-role": "foundations"},
					"tolerations": []map[string]interface{}{{
						"key":      "node-role",
						"operator": "Equal",
						"value":    "foundations",
						"effect":   "NoSchedule",
					}},
				},
			},
		},
	}
	if stateful {
		env.Charts["chainlink"].Values["db"] = map[string]interface{}{
			"stateful": true,
			"capacity": "50Gi",
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "2000m",
					"memory": "4000Mi",
				},
				"limits": map[string]interface{}{
					"cpu":    "2000m",
					"memory": "4000Mi",
				},
			},
		}
	}
	return env
}
