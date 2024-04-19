package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

var (
	feedLabels = []string{
		// This is the address of the account associated with one of the account names above.
		"account_address",
		"feed_id",
		"chain_id",
		"contract_status",
		"contract_type",
		"feed_name",
		"feed_path",
		"network_id",
		"network_name",
	}

	nodeLabels = []string{
		"account_address",
		"node_operator",
		"chain",
	}
)

var gauges map[string]*prometheus.GaugeVec

func makeBalanceMetricName(balanceAccountName string) string {
	return fmt.Sprintf("sol_balance_%s", balanceAccountName)
}

func init() {
	gauges = map[string]*prometheus.GaugeVec{}

	// initialize gauges for data feed accounts (state, transmissions, access controllers, etc)
	for _, balanceAccountName := range types.FeedBalanceAccountNames {
		gauges[balanceAccountName] = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: makeBalanceMetricName(balanceAccountName),
			},
			feedLabels,
		)
	}

	// init gauge for CL node balances
	gauges[types.NodeBalanceMetric] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: makeBalanceMetricName(types.NodeBalanceMetric),
		},
		nodeLabels,
	)

	// init gauges for tx details tracking
	for _, txDetailMetric := range types.TxDetailsMetrics {
		gauges[txDetailMetric] = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: txDetailMetric,
			},
			feedLabels,
		)
	}

	// init gauge for slot height
	gauges[types.SlotHeightMetric] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: types.SlotHeightMetric,
		},
		[]string{"chain", "url"},
	)
}

type FeedInput struct {
	AccountAddress, FeedID, ChainID, ContractStatus, ContractType, FeedName, FeedPath, NetworkID, NetworkName string
}

func (i FeedInput) ToPromLabels() prometheus.Labels {
	return prometheus.Labels{
		"account_address": i.AccountAddress,
		"feed_id":         i.FeedID,
		"chain_id":        i.ChainID,
		"contract_status": i.ContractStatus,
		"contract_type":   i.ContractType,
		"feed_name":       i.FeedName,
		"feed_path":       i.FeedPath,
		"network_id":      i.NetworkID,
		"network_name":    i.NetworkName,
	}
}
