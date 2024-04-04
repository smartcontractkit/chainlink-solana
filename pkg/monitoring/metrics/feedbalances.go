package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
)

//go:generate mockery --name FeedBalances --output ./mocks/

type FeedBalances interface {
	Exists(balanceAccountName string) (*prometheus.GaugeVec, bool)
	SetBalance(balance uint64, input FeedBalanceInput)
	Cleanup(input FeedBalanceInput)
}

var _ FeedBalances = (*feedBalances)(nil)

type feedBalances struct {
	log commonMonitoring.Logger
}

type FeedBalanceInput struct {
	BalanceAccountName, AccountAddress, FeedID, ChainID, ContractStatus, ContractType, FeedName, FeedPath, NetworkID, NetworkName string
}

func (i FeedBalanceInput) ToPromLabels() prometheus.Labels {
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

func NewFeedBalances(log commonMonitoring.Logger) *feedBalances {
	return &feedBalances{log}
}

func (fb *feedBalances) Exists(balanceAccountName string) (*prometheus.GaugeVec, bool) {
	g, ok := gauges[balanceAccountName]
	return g, ok
}

func (fb *feedBalances) SetBalance(balance uint64, input FeedBalanceInput) {
	gauge, found := fb.Exists(input.BalanceAccountName)
	if !found {
		panic(fmt.Sprintf("gauge not known for name '%s'", input.BalanceAccountName))
	}
	gauge.With(input.ToPromLabels()).Set(float64(balance))
}

func (fb *feedBalances) Cleanup(input FeedBalanceInput) {
	gauge, found := fb.Exists(input.BalanceAccountName)
	if !found {
		panic(fmt.Sprintf("gauge not known for name '%s'", input.BalanceAccountName))
	}
	gauge.Delete(input.ToPromLabels())
}
