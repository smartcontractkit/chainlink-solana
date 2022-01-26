package monitoring

import (
	"context"
	"sync"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewPrometheusExporterFactory(
	log relayMonitoring.Logger,
	metrics Metrics,
) relayMonitoring.ExporterFactory {
	return &prometheusExporterFactory{
		log,
		metrics,
	}
}

type prometheusExporterFactory struct {
	log     relayMonitoring.Logger
	metrics Metrics
}

func (p *prometheusExporterFactory) NewExporter(
	chainConfig relayMonitoring.ChainConfig,
	feedConfig relayMonitoring.FeedConfig,
) (relayMonitoring.Exporter, error) {
	return &prometheusExporter{
		chainConfig,
		feedConfig,
		p.log,
		p.metrics,
		sync.Mutex{},
		make(map[string]struct{}),
	}, nil
}

type prometheusExporter struct {
	chainConfig relayMonitoring.ChainConfig
	feedConfig  relayMonitoring.FeedConfig

	log     relayMonitoring.Logger
	metrics Metrics

	addressesMu  sync.Mutex
	addressesSet map[string]struct{}
}

func (p *prometheusExporter) Export(ctx context.Context, data interface{}) {
	balances, isBalances := data.(Balances)
	if !isBalances {
		return
	}
	for _, key := range BalanceAccountNames {
		address, okAddress := balances.Addresses[key]
		value, okValue := balances.Values[key]
		gauge, okGauge := gauges[key]
		if !okAddress || !okValue || !okGauge {
			p.log.Errorw("mismatch address and balance for key", "key", key, "address", address, "value", value, "gauge", gauge)
			continue
		}
		p.metrics.SetBalance(
			value,
			key,
			address.String(),
			p.feedConfig.GetContractAddress(),
			p.chainConfig.GetChainID(),
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
		)
	}
	p.addressesMu.Lock()
	defer p.addressesMu.Unlock()
	for _, address := range balances.Addresses {
		p.addressesSet[address.String()] = struct{}{}
	}
}

func (p *prometheusExporter) Cleanup(_ context.Context) {
	p.addressesMu.Lock()
	defer p.addressesMu.Unlock()
	for address := range p.addressesSet {
		p.metrics.Cleanup(
			address,
			p.feedConfig.GetContractAddress(),
			p.chainConfig.GetChainID(),
			p.feedConfig.GetContractStatus(),
			p.feedConfig.GetContractType(),
			p.feedConfig.GetName(),
			p.feedConfig.GetPath(),
			p.chainConfig.GetNetworkID(),
			p.chainConfig.GetNetworkName(),
		)
	}
}
