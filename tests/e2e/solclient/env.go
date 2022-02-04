package solclient

import (
	"github.com/smartcontractkit/helmenv/environment"
)

// NewChainlinkSolOCRv2 returns a cluster config with Solana test validator
func NewChainlinkSolOCRv2(nodes int, stateful bool) *environment.Config {
	var db map[string]interface{}
	if stateful {
		db = map[string]interface{}{
			"stateful": true,
			"capacity": "2Gi",
		}
	} else {
		db = map[string]interface{}{
			"stateful": false,
		}
	}
	return &environment.Config{
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
							"image":   "795953128386.dkr.ecr.us-west-2.amazonaws.com/chainlink",
							"version": "develop.latest",
						},
					},
					"db": db,
					"env": map[string]interface{}{
						"EVM_ENABLED":                 "false",
						"EVM_RPC_ENABLED":             "false",
						"SOLANA_ENABLED":              "true",
						"eth_url":                     "ws://sol:8900",
						"eth_disabled":                "true",
						"CHAINLINK_DEV":               "false",
						"USE_LEGACY_ETH_ENV_VARS":     "false",
						"FEATURE_OFFCHAIN_REPORTING2": "true",
						"feature_external_initiators": "true",
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
}
