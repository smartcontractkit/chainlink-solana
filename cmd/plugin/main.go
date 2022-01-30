package main

import (
	"github.com/hashicorp/go-plugin"

	relay "github.com/smartcontractkit/chainlink-relay/pkg/plugin"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/plugins"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func main() {
	lggr := logger.NewPluginLogger()
	defer lggr.Sync()
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: relay.SolanaHandshake,
		Plugins: map[string]plugin.Plugin{
			plugins.SolanaName: &relay.SolanaPlugin{
				Impl: solana.NewRelayer(lggr.Named("Solana")),
			},
		},
	})
}
