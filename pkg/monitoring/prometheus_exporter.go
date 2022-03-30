package monitoring

import (
	"context"
	"sync"

	"github.com/gagliardetto/solana-go"
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
		make(map[string]solana.PublicKey),
	}, nil
}

type prometheusExporter struct {
	chainConfig relayMonitoring.ChainConfig
	feedConfig  relayMonitoring.FeedConfig

	log     relayMonitoring.Logger
	metrics Metrics

	addressesMu sync.Mutex
	addresses   map[string]solana.PublicKey
}

func (p *prometheusExporter) Export(ctx context.Context, data interface{}) {
	balances, isBalances := data.(Balances)
	if !isBalances {
		return
	}
	for _, balanceAccountName := range BalanceAccountNames {
		address, okAddress := balances.Addresses[balanceAccountName]
		balance, okBalance := balances.Values[balanceAccountName]
		gauge, okGauge := gauges[balanceAccountName]
		if !okAddress || !okBalance || !okGauge {
			p.log.Errorw("mismatch address and balance for account name",
				"account-name", balanceAccountName,
				"address", address,
				"balance", balance,
				"gauge", gauge,
			)
			continue
		}
		p.metrics.SetBalance(
			balance,
			balanceAccountName,
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
	// Store the map of account names and their addresses for later cleanup.
	p.addressesMu.Lock()
	defer p.addressesMu.Unlock()
	p.addresses = balances.Addresses
}

func (p *prometheusExporter) Cleanup(_ context.Context) {
	p.addressesMu.Lock()
	defer p.addressesMu.Unlock()
	for balanceAccountName, address := range p.addresses {
		p.metrics.Cleanup(
			balanceAccountName,
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
}
