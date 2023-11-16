package monitoring

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	relayMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
)

var BalanceAccountNames = []string{
	"contract",
	"state",
	"transmissions",
	"token_vault",
	"requester_access_controller",
	"billing_access_controller",
}

var labelNames = []string{
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

var gauges map[string]*prometheus.GaugeVec

func makeMetricName(balanceAccountName string) string {
	return fmt.Sprintf("sol_balance_%s", balanceAccountName)
}

func init() {
	gauges = map[string]*prometheus.GaugeVec{}
	for _, balanceAccountName := range BalanceAccountNames {
		gauges[balanceAccountName] = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: makeMetricName(balanceAccountName),
			},
			labelNames,
		)
	}
}

type Metrics interface {
	SetBalance(balance uint64, balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
	Cleanup(balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
}

type defaultMetrics struct {
	log relayMonitoring.Logger
}

func NewMetrics(log relayMonitoring.Logger) Metrics {
	return &defaultMetrics{log}
}

func (d *defaultMetrics) SetBalance(balance uint64, balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	gauge, found := gauges[balanceAccountName]
	if !found {
		panic(fmt.Sprintf("gauge not known for name '%s'", balanceAccountName))
	}
	labels := prometheus.Labels{
		"account_address": accountAddress,
		"feed_id":         feedID,
		"chain_id":        chainID,
		"contract_status": contractStatus,
		"contract_type":   contractType,
		"feed_name":       feedName,
		"feed_path":       feedPath,
		"network_id":      networkID,
		"network_name":    networkName,
	}
	gauge.With(labels).Set(float64(balance))
}

func (d *defaultMetrics) Cleanup(balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	gauge, found := gauges[balanceAccountName]
	if !found {
		panic(fmt.Sprintf("gauge not known for name '%s'", balanceAccountName))
	}
	labels := prometheus.Labels{
		"account_address": accountAddress,
		"feed_id":         feedID,
		"chain_id":        chainID,
		"contract_status": contractStatus,
		"contract_type":   contractType,
		"feed_name":       feedName,
		"feed_path":       feedPath,
		"network_id":      networkID,
		"network_name":    networkName,
	}
	gauge.Delete(labels)
}
