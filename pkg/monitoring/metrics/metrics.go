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

	nodeFeedLabels = append([]string{
		"node_address",
		"node_operator",
	}, feedLabels...)

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

	// init gauge for node success per feed per node
	gauges[types.NodeSuccessMetric] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: types.NodeSuccessMetric,
		},
		nodeFeedLabels,
	)

	// init gauge for slot height
	gauges[types.SlotHeightMetric] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: types.SlotHeightMetric,
		},
		[]string{"chain", "url"},
	)

	// init gauge for network fees
	gauges[types.NetworkFeesMetric] = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: types.NetworkFeesMetric,
		},
		[]string{
			"type",      // compute budget price, total fee
			"operation", // avg, median, upper/lower quartile, min, max
			"chain",
		},
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

type NodeFeedInput struct {
	NodeAddress, NodeOperator string
	FeedInput
}

func (i NodeFeedInput) ToPromLabels() prometheus.Labels {
	l := i.FeedInput.ToPromLabels()
	l["node_address"] = i.NodeAddress
	l["node_operator"] = i.NodeOperator
	return l
}
