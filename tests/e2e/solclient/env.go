package solclient

import (
	"github.com/smartcontractkit/helmenv/environment"
)

// NewChainlinkSolOCRv2 returns a cluster config with Solana test validator
func NewChainlinkSolOCRv2(nodes int, stateful bool) *environment.Config {
	env := &environment.Config{
		NamespacePrefix: "chainlink-sol",
		Charts: environment.Charts{
			"solana-validator": {
				Index: 1,
				// TODO: remove these values when helm-env is properly setup with new version and the image is in chainlink ecr
				Values: map[string]interface{}{
					"sol": map[string]interface{}{
						"image": map[string]interface{}{
							"image":   "tateexon/solana-validator",
							"version": "1.9.14",
						},
					},
				},
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
							"image":   "public.ecr.aws/z0b1w9r9/chainlink",
							"version": "develop",
						},
					},
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
	if stateful {
		env.Charts["chainlink"].Values["db"] = map[string]interface{}{
			"stateful": true,
			"capacity": "2Gi",
		}
	}
	return env
}
