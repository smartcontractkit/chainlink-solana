package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestFeedBalances(t *testing.T) {
	m := NewFeedBalances(testutils.NewNullLogger())

	// fetching gauges
	bal, ok := m.Exists(types.FeedBalanceAccountNames[0])
	assert.NotNil(t, bal)
	assert.True(t, ok)
	missing, ok := m.Exists("this gauge should not exist")
	assert.Nil(t, missing)
	assert.False(t, ok)

	// setting gauges
	balanceAccountName := types.FeedBalanceAccountNames[0]
	input := FeedInput{
		AccountAddress: t.Name(), // use unique to prevent conflicts if run parallel
	}
	v := 100
	assert.NotPanics(t, func() { m.SetBalance(uint64(v), balanceAccountName, input) })
	promBal := testutil.ToFloat64(bal.With(input.ToPromLabels()))
	assert.Equal(t, float64(v), promBal)
	assert.Panics(t, func() { m.SetBalance(0, "", FeedInput{}) })

	// cleanup gauges
	assert.Equal(t, 1, testutil.CollectAndCount(bal))
	assert.NotPanics(t, func() { m.Cleanup(balanceAccountName, input) })
	assert.Equal(t, 0, testutil.CollectAndCount(bal))
	assert.Panics(t, func() { m.Cleanup("", FeedInput{}) })
}
