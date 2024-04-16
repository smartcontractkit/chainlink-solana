package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
)

//go:generate mockery --name FeedBalances --output ./mocks/

type FeedBalances interface {
	Exists(balanceAccountName string) (*prometheus.GaugeVec, bool)
	SetBalance(balance uint64, balanceAccountName string, feedInput FeedInput)
	Cleanup(balanceAccountName string, feedInput FeedInput)
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

func (fb *feedBalances) SetBalance(balance uint64, balanceAccountName string, feedInput FeedInput) {
	gauge, found := fb.Exists(balanceAccountName)
	if !found {
		panic(fmt.Sprintf("gauge not known for name '%s'", balanceAccountName))
	}
	gauge.With(feedInput.ToPromLabels()).Set(float64(balance))
}

func (fb *feedBalances) Cleanup(balanceAccountName string, feedInput FeedInput) {
	gauge, found := fb.Exists(balanceAccountName)
	if !found {
		panic(fmt.Sprintf("gauge not known for name '%s'", balanceAccountName))
	}
	gauge.Delete(feedInput.ToPromLabels())
}
