package monitoring

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewNodeBalancesSourceFactory(
	client ChainReader,
	log commonMonitoring.Logger,
) commonMonitoring.NetworkSourceFactory {
	return &nodeBalancesSourceFactory{
		client,
		log,
	}
}

type nodeBalancesSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (s *nodeBalancesSourceFactory) NewSource(_ commonMonitoring.ChainConfig, rddnodes []commonMonitoring.NodeConfig) (commonMonitoring.Source, error) {
	nodes, err := config.MakeSolanaNodeConfigs(rddnodes)
	if err != nil {
		return nil, fmt.Errorf("NodeBalancesSourceFactory.NewSource: %w", err)
	}

	nodesMap := map[string]solana.PublicKey{}
	for _, n := range nodes {
		pk, err := n.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to convert to public key: %w", err)
		}
		nodesMap[n.GetName()] = pk
	}

	return &balancesSource{
		s.client,
		s.log,
		nodesMap,
	}, nil
}

func (s *nodeBalancesSourceFactory) GetType() string {
	return types.BalanceType
}
