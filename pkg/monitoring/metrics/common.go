package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
)

// simpleGauge is an internal implementation for fetching a gauge from the gauges map
// and share logic for fetching, error handling, and setting.
// simpleGauge should be wrapped for export, not directly exported
type simpleGauge struct {
	log        commonMonitoring.Logger
	metricName string
}

func newSimpleGauge(log commonMonitoring.Logger, name string) simpleGauge {
	if log == nil {
		panic("simpleGauge.logger is nil")
	}
	return simpleGauge{log, name}
}

func (sg simpleGauge) run(
	f func(*prometheus.GaugeVec),
) {
	if gauges == nil {
		sg.log.Fatalw("gauges is nil")
		return
	}

	gauge, ok := gauges[sg.metricName]
	if !ok || gauge == nil {
		sg.log.Errorw("gauge not found", "name", sg.metricName)
		return
	}
	f(gauge)
}

func (sg simpleGauge) set(value float64, labels prometheus.Labels) {
	sg.run(func(g *prometheus.GaugeVec) { g.With(labels).Set(value) })
}

func (sg simpleGauge) delete(labels prometheus.Labels) {
	sg.run(func(g *prometheus.GaugeVec) { g.Delete(labels) })
}

func (sg simpleGauge) add(value float64, labels prometheus.Labels) {
	sg.run(func(g *prometheus.GaugeVec) { g.With(labels).Add(value) })
}
