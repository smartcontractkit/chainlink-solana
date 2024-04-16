package metrics

import (
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

//go:generate mockery --name ReportObservations --output ./mocks/

type ReportObservations interface {
	SetCount(count uint64, feedInput FeedInput)
	Cleanup(feedInput FeedInput)
}

var _ ReportObservations = (*reportObservations)(nil)

type reportObservations struct {
	simpleGauge
}

func NewReportObservations(log commonMonitoring.Logger) *reportObservations {
	return &reportObservations{newSimpleGauge(log, types.ReportObservationMetric)}
}

func (ro *reportObservations) SetCount(count uint64, feedInput FeedInput) {
	ro.set(float64(count), feedInput.ToPromLabels())
}

func (ro *reportObservations) Cleanup(feedInput FeedInput) {
	ro.delete(feedInput.ToPromLabels())
}
