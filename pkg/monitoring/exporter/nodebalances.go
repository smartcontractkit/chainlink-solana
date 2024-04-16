package exporter

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

type metricsBuilder func(commonMonitoring.Logger, string) metrics.NodeBalances

func NewNodeBalancesFactory(log commonMonitoring.Logger, metricsFunc metricsBuilder) commonMonitoring.ExporterFactory {
	return &nodeBalancesFactory{
		log,
		metricsFunc,
	}

}

type nodeBalancesFactory struct {
	log         commonMonitoring.Logger
	metricsFunc metricsBuilder
}

func (f *nodeBalancesFactory) NewExporter(params commonMonitoring.ExporterParams) (commonMonitoring.Exporter, error) {
	if f.metricsFunc == nil {
		return nil, fmt.Errorf("metrics generator is nil")
	}
	return &nodeBalances{
		log:     f.log,
		metrics: metrics.NewNodeBalances(f.log, params.ChainConfig.GetNetworkName()),
	}, nil
}

type nodeBalances struct {
	log     commonMonitoring.Logger
	metrics metrics.NodeBalances

	lock      sync.Mutex
	addresses map[string]solana.PublicKey
}

func (nb *nodeBalances) Export(ctx context.Context, data interface{}) {
	balances, isBalances := data.(types.Balances)
	if !isBalances {
		return
	}
	for operator, address := range balances.Addresses {
		balance, ok := balances.Values[operator]
		if !ok {
			nb.log.Errorw("mismatch addresses and balances",
				"operator", operator,
				"address", address,
			)
			continue
		}
		nb.metrics.SetBalance(balance, address.String(), operator)
	}

	nb.lock.Lock()
	defer nb.lock.Unlock()
	nb.addresses = balances.Addresses
}

func (nb *nodeBalances) Cleanup(_ context.Context) {
	nb.lock.Lock()
	defer nb.lock.Unlock()
	for operator, address := range nb.addresses {
		nb.metrics.Cleanup(address.String(), operator)
	}
}
