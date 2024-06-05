package metrics

import (
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestNetworkFees(t *testing.T) {
	lgr := logger.Test(t)
	m := NewNetworkFees(lgr)

	// fetching gauges
	g, ok := gauges[types.NetworkFeesMetric]
	require.True(t, ok)

	input := NetworkFeesInput{}
	chain := t.Name() + "_chain"
	ind := 0
	for _, t := range []string{"fee", "computeUnitPrice"} {
		for _, o := range []string{"median", "avg"} {
			ind++
			input.Set(t, o, uint64(ind)) // 1..4
		}
	}

	// set gauge
	var values []int
	assert.NotPanics(t, func() { m.Set(input, chain) })
	// check values
	for _, l := range input.Labels(chain) {
		promBal := testutil.ToFloat64(g.With(l))
		values = append(values, int(promBal))
	}
	for i := 1; i <= ind; i++ {
		assert.True(t, slices.Contains(values, i))
	}

	// cleanup gauges
	assert.Equal(t, ind, testutil.CollectAndCount(g))
	assert.NotPanics(t, func() { m.Cleanup() })
	assert.Equal(t, 0, testutil.CollectAndCount(g))
}
