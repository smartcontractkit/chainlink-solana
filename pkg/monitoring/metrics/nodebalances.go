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
	log   commonMonitoring.Logger
	chain string
}

func NewNodeBalances(log commonMonitoring.Logger, chain string) NodeBalances {
	return &nodeBalances{log, chain}
}

func (nb *nodeBalances) SetBalance(balance uint64, address, operator string) {
	gauge, ok := gauges[types.NodeBalanceMetric]
	if !ok {
		nb.log.Fatalw("gauge not found", "name", types.NodeBalanceMetric)
		return
	}

	gauge.With(prometheus.Labels{
		"account_address": address,
		"node_operator":   operator,
		"chain":           nb.chain,
	}).Set(float64(balance))
}

func (nb *nodeBalances) Cleanup(address, operator string) {
	gauge, ok := gauges[types.NodeBalanceMetric]
	if !ok {
		nb.log.Fatalw("gauge not found", "name", types.NodeBalanceMetric)
		return
	}

	gauge.Delete(prometheus.Labels{
		"account_address": address,
		"node_operator":   operator,
		"chain":           nb.chain,
	})
}
