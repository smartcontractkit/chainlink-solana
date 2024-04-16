package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestReportObservations(t *testing.T) {
	lgr := logger.Test(t)
	m := NewReportObservations(lgr)

	// fetching gauges
	g, ok := gauges[types.ReportObservationMetric]
	require.True(t, ok)

	v := 100
	inputs := FeedInput{NetworkName: t.Name()}

	// set gauge
	assert.NotPanics(t, func() { m.SetCount(uint64(v), inputs) })
	promBal := testutil.ToFloat64(g.With(inputs.ToPromLabels()))
	assert.Equal(t, float64(v), promBal)

	// cleanup gauges
	assert.Equal(t, 1, testutil.CollectAndCount(g))
	assert.NotPanics(t, func() { m.Cleanup(inputs) })
	assert.Equal(t, 0, testutil.CollectAndCount(g))
}
