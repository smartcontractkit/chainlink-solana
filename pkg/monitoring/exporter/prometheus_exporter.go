package exporter

import (
	"context"
	"sync"

	"github.com/gagliardetto/solana-go"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewFeedBalancePrometheusExporterFactory(
	log commonMonitoring.Logger,
	metrics metrics.Metrics,
) commonMonitoring.ExporterFactory {
	return &feedBalancePrometheusExporterFactory{
		log,
		metrics,
	}
}

type feedBalancePrometheusExporterFactory struct {
	log     commonMonitoring.Logger
	metrics metrics.Metrics
}

func (p *feedBalancePrometheusExporterFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	return &feedBalancePrometheusExporter{
		params.ChainConfig,
		params.FeedConfig,
		p.log,
		p.metrics,
		sync.Mutex{},
		make(map[string]solana.PublicKey),
	}, nil
}

type feedBalancePrometheusExporter struct {
	chainConfig commonMonitoring.ChainConfig
	feedConfig  commonMonitoring.FeedConfig

	log     commonMonitoring.Logger
	metrics metrics.Metrics

	addressesMu sync.Mutex
	addresses   map[string]solana.PublicKey
}

func (p *feedBalancePrometheusExporter) Export(ctx context.Context, data interface{}) {
	balances, isBalances := data.(types.Balances)
	if !isBalances {
		return
	}
	for _, balanceAccountName := range types.FeedBalanceAccountNames {
		address, okAddress := balances.Addresses[balanceAccountName]
		balance, okBalance := balances.Values[balanceAccountName]
		gauge, okGauge := metrics.Gauges[balanceAccountName]
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

func (p *feedBalancePrometheusExporter) Cleanup(_ context.Context) {
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
