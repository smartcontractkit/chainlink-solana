package solclient

import (
	"github.com/smartcontractkit/helmenv/environment"
)

// NewChainlinkSolOCRv2 returns a cluster config with Solana test validator
func NewChainlinkSolOCRv2() *environment.Config {
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
					"replicas": 5,
					"chainlink": map[string]interface{}{
						"image": map[string]interface{}{
							"image":   "public.ecr.aws/chainlink/chainlink",
							"version": "develop.426eb924464697f714f14e1e0c7804fca09977b8",
						},
					},
					"env": map[string]interface{}{
						"eth_url":                     "ws://sol:8900",
						"eth_disabled":                "true",
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
