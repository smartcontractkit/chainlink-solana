package monitoring

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

func NewNetworkFeesSourceFactory(
	client ChainReader,
	log commonMonitoring.Logger,
) commonMonitoring.NetworkSourceFactory {
	return &networkFeesSourceFactory{
		client,
		log,
	}
}

type networkFeesSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (s *networkFeesSourceFactory) NewSource(
	_ commonMonitoring.ChainConfig,
	_ []commonMonitoring.NodeConfig,
) (commonMonitoring.Source, error) {
	return &networkFeesSource{s.client}, nil
}

func (s *networkFeesSourceFactory) GetType() string {
	return types.NetworkFeesType
}

type networkFeesSource struct {
	client ChainReader
}

func (t *networkFeesSource) Fetch(ctx context.Context) (interface{}, error) {
	block, err := t.client.GetLatestBlock(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, err
	}
	return fees.ParseBlock(block) // return fees.BlockData, err
}
