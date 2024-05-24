package metrics

import (
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

//go:generate mockery --name NodeSuccess --output ./mocks/

type NodeSuccess interface {
	Add(count int, i NodeFeedInput)
	Cleanup(i NodeFeedInput)
}

var _ NodeSuccess = (*nodeSuccess)(nil)

type nodeSuccess struct {
	simpleGauge
}

func NewNodeSuccess(log commonMonitoring.Logger) *nodeSuccess {
	return &nodeSuccess{newSimpleGauge(log, types.NodeSuccessMetric)}
}

func (ro *nodeSuccess) Add(count int, i NodeFeedInput) {
	ro.add(float64(count), i.ToPromLabels())
}

func (ro *nodeSuccess) Cleanup(i NodeFeedInput) {
	ro.delete(i.ToPromLabels())
}
