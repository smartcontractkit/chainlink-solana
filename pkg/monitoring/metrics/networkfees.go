package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

//go:generate mockery --name NetworkFees --output ./mocks/

type NetworkFees interface {
	Set(slot NetworkFeesInput, chain string)
	Cleanup()
}

var _ NetworkFees = (*networkFees)(nil)

type networkFees struct {
	simpleGauge
	labels []prometheus.Labels
}

func NewNetworkFees(log commonMonitoring.Logger) *networkFees {
	return &networkFees{
		simpleGauge: newSimpleGauge(log, types.NetworkFeesMetric),
	}
}

func (sh *networkFees) Set(input NetworkFeesInput, chain string) {
	for feeType, opMap := range input {
		for operation, value := range opMap {
			label := prometheus.Labels{
				"type":      feeType,
				"operation": operation,
				"chain":     chain,
			}
			sh.set(float64(value), label)
		}
	}
	sh.labels = input.Labels(chain)
}

func (sh *networkFees) Cleanup() {
	for _, l := range sh.labels {
		sh.delete(l)
	}
}

type NetworkFeesInput map[string]map[string]uint64

func (i NetworkFeesInput) Set(feeType, operation string, value uint64) {
	if _, exists := i[feeType]; !exists {
		i[feeType] = map[string]uint64{}
	}
	i[feeType][operation] = value
}

func (i NetworkFeesInput) Labels(chain string) (l []prometheus.Labels) {
	for feeType, opMap := range i {
		for operation := range opMap {
			l = append(l, prometheus.Labels{
				"type":      feeType,
				"operation": operation,
				"chain":     chain,
			})
		}
	}
	return
}
