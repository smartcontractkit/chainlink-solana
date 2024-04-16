package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

//go:generate mockery --name NodeBalances --output ./mocks/

type NodeBalances interface {
	SetBalance(balance uint64, address, operator string)
	Cleanup(address, operator string)
}

type nodeBalances struct {
	simpleGauge
	chain string
}

func NewNodeBalances(log commonMonitoring.Logger, chain string) NodeBalances {
	return &nodeBalances{
		newSimpleGauge(log, types.NodeBalanceMetric),
		chain,
	}
}

func (nb *nodeBalances) SetBalance(balance uint64, address, operator string) {
	nb.set(float64(balance), prometheus.Labels{
		"account_address": address,
		"node_operator":   operator,
		"chain":           nb.chain,
	})
}

func (nb *nodeBalances) Cleanup(address, operator string) {
	nb.delete(prometheus.Labels{
		"account_address": address,
		"node_operator":   operator,
		"chain":           nb.chain,
	})
}
