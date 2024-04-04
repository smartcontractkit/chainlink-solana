package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
)

//go:generate mockery --name FeedBalances --output ./mocks/

type FeedBalances interface {
	Exists(balanceAccountName string) (*prometheus.GaugeVec, bool)
	SetBalance(balance uint64, balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
	Cleanup(balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string)
}

var _ FeedBalances = (*feedBalances)(nil)

type feedBalances struct {
	log commonMonitoring.Logger
}

func NewFeedBalances(log commonMonitoring.Logger) *feedBalances {
	return &feedBalances{log}
}

func (fb *feedBalances) Exists(balanceAccountName string) (*prometheus.GaugeVec, bool) {
	g, ok := gauges[balanceAccountName]
	return g, ok
}

func (fb *feedBalances) SetBalance(balance uint64, balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	gauge, found := fb.Exists(balanceAccountName)
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

func (fb *feedBalances) Cleanup(balanceAccountName, accountAddress, feedID, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
	gauge, found := fb.Exists(balanceAccountName)
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
