package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/pelletier/go-toml/v2"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/loop"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	solcfg "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const (
	loggerName = "PluginSolana"
)

func main() {
	s := loop.MustNewStartedServer(loggerName)
	defer s.Stop()

	p := &pluginRelayer{Plugin: loop.Plugin{Logger: s.Logger}, ds: s.DataSource}
	defer s.Logger.ErrorIfFn(p.Close, "Failed to close")

	s.MustRegister(p)

	stopCh := make(chan struct{})
	defer close(stopCh)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: loop.PluginRelayerHandshakeConfig(),
		Plugins: map[string]plugin.Plugin{
			loop.PluginRelayerName: &loop.GRPCPluginRelayer{
				PluginServer: p,
				BrokerConfig: loop.BrokerConfig{
					StopCh:   stopCh,
					Logger:   s.Logger,
					GRPCOpts: s.GRPCOpts,
				},
			},
		},
		GRPCServer: s.GRPCOpts.NewServer,
	})
}

type pluginRelayer struct {
	loop.Plugin
	ds sqlutil.DataSource
}

func (c *pluginRelayer) NewRelayer(ctx context.Context, config string, keystore core.Keystore, csaKeystore core.Keystore, capRegistry core.CapabilitiesRegistry) (loop.Relayer, error) {
	d := toml.NewDecoder(strings.NewReader(config))
	d.DisallowUnknownFields()
	var cfg struct {
		Solana solcfg.TOMLConfig
	}

	if err := d.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config toml: %w:\n\t%s", err, config)
	}

	rawNodes := make([]map[string]string, 0, len(cfg.Solana.Nodes))
	for _, n := range cfg.Solana.Nodes {
		if n == nil || n.URL == nil {
			continue
		}
		rawNodes = append(rawNodes, map[string]string{"URL": n.URL.String()})
	}
	chainID := ""
	if cfg.Solana.ChainID != nil {
		chainID = *cfg.Solana.ChainID
	}
	emitter := loop.NewPluginRelayerConfigEmitter(
		c.Logger,
		beholder.GetClient().Config.AuthPublicKeyHex,
		chainID,
		rawNodes,
	)
	if err := emitter.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start plugin relayer config emitter: %w", err)
	}
	c.SubService(emitter)

	opts := solana.ChainOpts{
		Logger:   c.Logger,
		KeyStore: keystore,
		DS:       c.ds,
	}

	chain, err := solana.NewChain(&cfg.Solana, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain: %w", err)
	}

	ra := solana.NewRelayer(c.Logger, chain, capRegistry)

	c.SubService(ra)

	return ra, nil
}
