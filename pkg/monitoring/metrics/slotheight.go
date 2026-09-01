package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	frameworkMetrics "github.com/smartcontractkit/chainlink-framework/metrics"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

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
	// Sanitize the URL to avoid leaking credentials (e.g. API keys embedded in
	// the path/query or userinfo of managed RPC endpoints) into metric labels.
	sh.labels = prometheus.Labels{"chain": chain, "url": frameworkMetrics.SanitizeRPCURL(url)}
	sh.set(float64(slot), sh.labels)
}

func (sh *slotHeight) Cleanup() {
	sh.delete(sh.labels)
}
