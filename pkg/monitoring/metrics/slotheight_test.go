package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestSlotHeight(t *testing.T) {
	lgr := logger.Test(t)
	m := NewSlotHeight(lgr)

	// fetching gauges
	g, ok := gauges[types.SlotHeightMetric]
	require.True(t, ok)

	v := 100

	// set gauge
	assert.NotPanics(t, func() { m.Set(types.SlotHeight(v), t.Name(), t.Name()+"_url") })
	promBal := testutil.ToFloat64(g.With(prometheus.Labels{
		"chain": t.Name(),
		"url":   t.Name() + "_url",
	}))
	assert.Equal(t, float64(v), promBal)

	// cleanup gauges
	assert.Equal(t, 1, testutil.CollectAndCount(g))
	assert.NotPanics(t, func() { m.Cleanup() })
	assert.Equal(t, 0, testutil.CollectAndCount(g))
}
