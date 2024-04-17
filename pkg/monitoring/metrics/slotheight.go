package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

//go:generate mockery --name SlotHeight --output ./mocks/

type SlotHeight interface {
	Set(slot types.SlotHeight, chain, url string)
	Cleanup()
}

var _ SlotHeight = (*slotHeight)(nil)

type slotHeight struct {
	simpleGauge
	labels prometheus.Labels
}

func NewSlotHeight(log commonMonitoring.Logger) *slotHeight {
	return &slotHeight{
		simpleGauge: newSimpleGauge(log, types.SlotHeightMetric),
	}
}

func (sh *slotHeight) Set(slot types.SlotHeight, chain, url string) {
	sh.labels = prometheus.Labels{"chain": chain, "url": url}
	sh.set(float64(slot), sh.labels)
}

func (sh *slotHeight) Cleanup() {
	sh.delete(sh.labels)
}
