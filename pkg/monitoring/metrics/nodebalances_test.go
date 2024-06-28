package metrics

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestNodeBalances(t *testing.T) {
	m := NewNodeBalances(testutils.NewNullLogger(), t.Name())

	// fetching gauges
	bal, ok := gauges[types.NodeBalanceMetric]
	require.True(t, ok)

	v := 100
	addr := solana.PublicKey{1}.String()
	operator := t.Name() + "-feed"
	label := prometheus.Labels{
		"account_address": addr,
		"node_operator":   operator,
		"chain":           t.Name(),
	}

	// set gauge
	assert.NotPanics(t, func() { m.SetBalance(uint64(v), addr, operator) })
	promBal := testutil.ToFloat64(bal.With(label))
	assert.Equal(t, float64(v), promBal)

	// cleanup gauges
	assert.Equal(t, 1, testutil.CollectAndCount(bal))
	assert.NotPanics(t, func() { m.Cleanup(addr, operator) })
	assert.Equal(t, 0, testutil.CollectAndCount(bal))
}
