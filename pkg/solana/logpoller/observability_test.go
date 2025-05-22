package logpoller

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	ioprometheusclient "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/sqltest"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-framework/metrics"
)

const chainFamily = "Solana Test"

func TestShouldPublishDurationInCaseOfError(t *testing.T) {
	sqltest.SkipInMemory(t)
	ctx := t.Context()
	orm := createObservedORM(t, "testChainID")
	t.Cleanup(func() { resetMetrics(*orm) })

	require.Equal(t, 0, testutil.CollectAndCount(orm.queryDuration))

	// Cancel ctx to force error
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err := orm.FilteredLogs(ctx, nil, query.LimitAndSort{}, "")
	require.Error(t, err)
	require.Equal(t, 1, testutil.CollectAndCount(orm.queryDuration))
}

func TestMetricsAreProperlyPopulatedWithLabels(t *testing.T) {
	orm := createObservedORM(t, chainID)
	t.Cleanup(func() { resetMetrics(*orm) })
	expectedCount := 9
	expectedSize := 2

	for i := 0; i < expectedCount; i++ {
		_, err := withObservedQueryAndResults(orm, "query", func() ([]string, error) { return []string{"value1", "value2"}, nil })
		require.NoError(t, err)
	}

	require.Equal(t, expectedCount, counterFromHistogramByLabels(t, orm.queryDuration, chainFamily, chainID, "query", "read"))
	require.Equal(t, expectedSize, counterFromGaugeByLabels(orm.datasetSize, chainFamily, chainID, "query", "read"))

	require.Equal(t, 0, counterFromHistogramByLabels(t, orm.queryDuration, chainFamily, chainID, "other_query", "read"))
	require.Equal(t, 0, counterFromHistogramByLabels(t, orm.queryDuration, chainFamily, "5", "query", "read"))

	require.Equal(t, 0, counterFromGaugeByLabels(orm.datasetSize, chainFamily, chainID, "other_query", "read"))
	require.Equal(t, 0, counterFromGaugeByLabels(orm.datasetSize, chainFamily, "5", "query", "read"))
}

func TestNotPublishingDatasetSizeInCaseOfError(t *testing.T) {
	orm := createObservedORM(t, chainID)
	t.Cleanup(func() { resetMetrics(*orm) })

	_, err := withObservedQueryAndResults(orm, "errorQuery", func() ([]string, error) { return nil, fmt.Errorf("error") })
	require.Error(t, err)

	require.Equal(t, 1, counterFromHistogramByLabels(t, orm.queryDuration, chainFamily, chainID, "errorQuery", "read"))
	require.Equal(t, 0, counterFromGaugeByLabels(orm.datasetSize, chainFamily, chainID, "errorQuery", "read"))
}

func TestMetricsAreProperlyPopulatedForWrites(t *testing.T) {
	orm := createObservedORM(t, chainID)
	t.Cleanup(func() { resetMetrics(*orm) })

	require.NoError(t, withObservedExec(orm, "execQuery", metrics.Create, func() error { return nil }))
	require.Error(t, withObservedExec(orm, "execQuery", metrics.Create, func() error { return fmt.Errorf("error") }))
	require.Equal(t, 2, counterFromHistogramByLabels(t, orm.queryDuration, chainFamily, chainID, "execQuery", "create"))
}

func TestCountersAreProperlyPopulatedForWrites(t *testing.T) {
	sqltest.SkipInMemory(t)

	ctx := t.Context()
	orm := createObservedORM(t, chainID)
	t.Cleanup(func() { resetMetrics(*orm) })

	filterID, err := orm.InsertFilter(t.Context(), newRandomFilter(t))
	require.NoError(t, err)

	logs := generateRandomLogs(t, filterID, 20)

	// First insert 10 logs
	require.NoError(t, orm.InsertLogs(ctx, logs[:10]))
	assert.Equal(t, float64(10), testutil.ToFloat64(metrics.PromLpLogsInserted.WithLabelValues(chainFamily, chainID)))
	assert.Equal(t, float64(10), testutil.ToFloat64(orm.logsInserted.WithLabelValues(chainFamily, chainID)))

	// Insert 5 more logs
	require.NoError(t, orm.InsertLogs(ctx, logs[10:15]))
	assert.Equal(t, float64(15), testutil.ToFloat64(orm.logsInserted.WithLabelValues(chainFamily, chainID)))

	// Insert 5 more logs
	require.NoError(t, orm.InsertLogs(ctx, logs[15:]))
	assert.Equal(t, float64(20), testutil.ToFloat64(orm.logsInserted.WithLabelValues(chainFamily, chainID)))
}

func generateRandomLogs(t *testing.T, filterID int64, count int) []types.Log {
	logs := make([]types.Log, count)
	for i := range logs {
		logs[i] = newRandomLog(t, filterID, chainID, "My Event")
	}
	return logs
}

func createObservedORM(t *testing.T, chainID string) *ObservedORM {
	lggr := logger.Test(t)
	db := sqltest.NewDB(t, sqltest.TestURL(t))
	orm, err := NewObservedORM(chainID, chainFamily, db, lggr)
	require.NoError(t, err)
	return orm
}

func resetMetrics(lp ObservedORM) {
	lp.queryDuration.Reset()
	lp.datasetSize.Reset()
	lp.logsInserted.Reset()
}

func counterFromGaugeByLabels(gaugeVec *prometheus.GaugeVec, labels ...string) int {
	value := testutil.ToFloat64(gaugeVec.WithLabelValues(labels...))
	return int(value)
}

func counterFromHistogramByLabels(t *testing.T, histogramVec *prometheus.HistogramVec, labels ...string) int {
	observer, err := histogramVec.GetMetricWithLabelValues(labels...)
	require.NoError(t, err)

	metricCh := make(chan prometheus.Metric, 1)
	observer.(prometheus.Histogram).Collect(metricCh)
	close(metricCh)

	metric := <-metricCh
	pb := &ioprometheusclient.Metric{}
	err = metric.Write(pb)
	require.NoError(t, err)

	return int(pb.GetHistogram().GetSampleCount())
}
