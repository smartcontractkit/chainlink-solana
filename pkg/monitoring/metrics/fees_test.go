package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

func TestFees(t *testing.T) {
	lgr := logger.Test(t)
	m := NewFees(lgr)

	// fetching gauges
	gFees, ok := gauges[types.TxFeeMetric]
	require.True(t, ok)
	gComputeUnits, ok := gauges[types.ComputeUnitPriceMetric]
	require.True(t, ok)

	v0 := 1
	v1 := 10
	l := FeedInput{NetworkID: t.Name()}

	// set gauge
	assert.NotPanics(t, func() {
		m.Set(uint64(v0), fees.ComputeUnitPrice(v1), l)
	})
	num := testutil.ToFloat64(gFees.With(l.ToPromLabels()))
	assert.Equal(t, float64(v0), num)
	num = testutil.ToFloat64(gComputeUnits.With(l.ToPromLabels()))
	assert.Equal(t, float64(v1), num)

	// cleanup gauges
	assert.Equal(t, 1, testutil.CollectAndCount(gFees))
	assert.Equal(t, 1, testutil.CollectAndCount(gComputeUnits))
	assert.NotPanics(t, func() { m.Cleanup(l) })
	assert.Equal(t, 0, testutil.CollectAndCount(gFees))
	assert.Equal(t, 0, testutil.CollectAndCount(gComputeUnits))
}
