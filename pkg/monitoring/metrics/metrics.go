package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

var (
	feedBalanceLabelNames = []string{
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

	nodeBalanceLabels = []string{
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
			feedBalanceLabelNames,
		)
	}

	// init gauge for CL node balances
	gauges[types.NodeBalanceMetric] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: makeBalanceMetricName(types.NodeBalanceMetric),
		},
		nodeBalanceLabels,
	)
}
