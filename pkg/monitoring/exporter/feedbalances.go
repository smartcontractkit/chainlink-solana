package exporter

import (
	"context"
	"sync"

	"github.com/gagliardetto/solana-go"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewFeedBalancesFactory(
	log commonMonitoring.Logger,
	metrics metrics.FeedBalances,
) commonMonitoring.ExporterFactory {
	return &feedBalancesFactory{
		log,
		metrics,
	}
}

type feedBalancesFactory struct {
	log     commonMonitoring.Logger
	metrics metrics.FeedBalances
}

func (p *feedBalancesFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	return &feedBalances{
		params.ChainConfig,
		params.FeedConfig,
		p.log,
		p.metrics,
		sync.Mutex{},
		make(map[string]solana.PublicKey),
	}, nil
}

type feedBalances struct {
	chainConfig commonMonitoring.ChainConfig
	feedConfig  commonMonitoring.FeedConfig

	log     commonMonitoring.Logger
	metrics metrics.FeedBalances

	addressesMu sync.Mutex
	addresses   map[string]solana.PublicKey
}

func (p *feedBalances) Export(ctx context.Context, data interface{}) {
	balances, isBalances := data.(types.Balances)
	if !isBalances {
		return
	}
	for _, balanceAccountName := range types.FeedBalanceAccountNames {
		address, okAddress := balances.Addresses[balanceAccountName]
		balance, okBalance := balances.Values[balanceAccountName]
		gauge, okGauge := p.metrics.Exists(balanceAccountName)
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
			metrics.FeedInput{
				AccountAddress: address.String(),
				FeedID:         p.feedConfig.GetContractAddress(),
				ChainID:        p.chainConfig.GetChainID(),
				ContractStatus: p.feedConfig.GetContractStatus(),
				ContractType:   p.feedConfig.GetContractType(),
				FeedName:       p.feedConfig.GetName(),
				FeedPath:       p.feedConfig.GetPath(),
				NetworkID:      p.chainConfig.GetNetworkID(),
				NetworkName:    p.chainConfig.GetNetworkName(),
			},
		)
	}
	// Store the map of account names and their addresses for later cleanup.
	p.addressesMu.Lock()
	defer p.addressesMu.Unlock()
	p.addresses = balances.Addresses
}

func (p *feedBalances) Cleanup(_ context.Context) {
	p.addressesMu.Lock()
	defer p.addressesMu.Unlock()
	for balanceAccountName, address := range p.addresses {
		p.metrics.Cleanup(balanceAccountName, metrics.FeedInput{
			AccountAddress: address.String(),
			FeedID:         p.feedConfig.GetContractAddress(),
			ChainID:        p.chainConfig.GetChainID(),
			ContractStatus: p.feedConfig.GetContractStatus(),
			ContractType:   p.feedConfig.GetContractType(),
			FeedName:       p.feedConfig.GetName(),
			FeedPath:       p.feedConfig.GetPath(),
			NetworkID:      p.chainConfig.GetNetworkID(),
			NetworkName:    p.chainConfig.GetNetworkName(),
		})
	}
}
