package api

import (
	"errors"
	"time"

	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type RelayerConfigBuilder struct {
	nodesBuilder []NodeConfigBuilder
	chainID      *string
	logger       *logger.Logger
}

func NewRelayerConfigBuilder() *RelayerConfigBuilder {
	return &RelayerConfigBuilder{}
}

func (rcb *RelayerConfigBuilder) WithNode(nodeBuilder *NodeConfigBuilder) *RelayerConfigBuilder {
	rcb.nodesBuilder = append(rcb.nodesBuilder, *nodeBuilder)
	return rcb
}

func (rcb *RelayerConfigBuilder) WithChainID(chainID string) *RelayerConfigBuilder {
	rcb.chainID = &chainID
	return rcb
}

func (rcb RelayerConfigBuilder) Build() (SolanaRelayerConfig, error) {
	if len(rcb.nodesBuilder) == 0 {
		return SolanaRelayerConfig{}, errors.New("Cannot create a config without solana validator nodes to connect to")
	}
	if rcb.chainID == nil {
		return SolanaRelayerConfig{}, errors.New("Cannot create a config without a ChainID")
	}
	var _logger logger.Logger
	if rcb.logger == nil {
		var err error
		_logger, err = logger.New()
		if err != nil {
			return SolanaRelayerConfig{}, err
		}
	} else {
		_logger = *rcb.logger
	}
	chainConfig := config.NewDefault()
	duration, _ := commonconfig.NewDuration(time.Hour)
	chainConfig.SetFrom(&config.TOMLConfig{
		Chain: config.Chain{
			TxRetentionTimeout: &duration,
		},
	})
	for _, nodeBuilder := range rcb.nodesBuilder {
		node, err := nodeBuilder.build()
		if err != nil {
			return SolanaRelayerConfig{}, err
		}
		chainConfig.Nodes = append(chainConfig.Nodes, &node)
	}
	chainConfig.ChainID = rcb.chainID

	relayerConfig := SolanaRelayerConfig{
		Logger:     &_logger,
		TOMLConfig: *chainConfig,
	}
	return relayerConfig, nil

}

type NodeConfigBuilder struct {
	url *commonconfig.URL
}

func NewNodeConfigBuilder() *NodeConfigBuilder {
	return &NodeConfigBuilder{}
}

func (ncb *NodeConfigBuilder) WithURL(URL commonconfig.URL) *NodeConfigBuilder {
	ncb.url = &URL
	return ncb
}

func (ncb NodeConfigBuilder) build() (config.Node, error) {
	if ncb.url == nil {
		return config.Node{}, errors.New("Cannot create a Node without a URL")
	}
	name := ncb.url.String()
	return config.Node{
		URL:  ncb.url,
		Name: &name,
	}, nil
}
