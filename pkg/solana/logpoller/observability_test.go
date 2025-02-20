package logpoller

import (
	"fmt"
	"github.com/google/uuid"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/sqltest"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestMultipleMetricsArePublished(t *testing.T) {
	ctx := tests.Context(t)
	chainID := uuid.NewString()
	orm := createObservedORM(t, chainID)
	t.Cleanup(func() { resetMetrics(*orm) })
	require.Equal(t, 0, testutil.CollectAndCount(orm.queryDuration))

	filter := newRandomFilter(t)
	filterID, err := orm.InsertFilter(ctx, filter)
	require.NoError(t, err)

	filter = newRandomFilter(t)
	_, err = orm.InsertFilter(ctx, filter)
	require.NoError(t, err)

	for i := 0; i < 20; i++ {
		log := newRandomLog(t, filterID, chainID, "My Event")
		err = orm.InsertLogs(ctx, []Log{log})
		require.NoError(t, err)
	}
	_, _ = orm.SelectSeqNums(ctx)

	require.Equal(t, 3, testutil.CollectAndCount(orm.queryDuration))
	require.Equal(t, 21, testutil.CollectAndCount(orm.datasetSize))
}

func TestShouldPublishDurationInCaseOfError(t *testing.T) {
	ctx := tests.Context(t)
	orm := createObservedORM(t, "testChainID")
	t.Cleanup(func() { resetMetrics(*orm) })
	require.Equal(t, 0, testutil.CollectAndCount(orm.queryDuration))

	// TODO: how an I make this test useful?
	_, err := orm.FilteredLogs(ctx, []query.Expression{}, query.LimitAndSort{}, "")
	require.Error(t, err)

	require.Equal(t, 1, testutil.CollectAndCount(orm.queryDuration))
	require.Equal(t, 1, counterFromHistogramByLabels(t, orm.queryDuration, "200", "FilteredLogs", "read"))
}

func TestMetricsAreProperlyPopulatedWithLabels(t *testing.T) {
	orm := createObservedORM(t, "420")
	t.Cleanup(func() { resetMetrics(*orm) })
	expectedCount := 9
	expectedSize := 2

	for i := 0; i < expectedCount; i++ {
		_, err := withObservedQueryAndResults(orm, "query", func() ([]string, error) { return []string{"value1", "value2"}, nil })
		require.NoError(t, err)
	}

	require.Equal(t, expectedCount, counterFromHistogramByLabels(t, orm.queryDuration, "420", "query", "read"))
	require.Equal(t, expectedSize, counterFromGaugeByLabels(orm.datasetSize, "420", "query", "read"))

	require.Equal(t, 0, counterFromHistogramByLabels(t, orm.queryDuration, "420", "other_query", "read"))
	require.Equal(t, 0, counterFromHistogramByLabels(t, orm.queryDuration, "5", "query", "read"))

	require.Equal(t, 0, counterFromGaugeByLabels(orm.datasetSize, "420", "other_query", "read"))
	require.Equal(t, 0, counterFromGaugeByLabels(orm.datasetSize, "5", "query", "read"))
}

func TestNotPublishingDatasetSizeInCaseOfError(t *testing.T) {
	orm := createObservedORM(t, "420")

	_, err := withObservedQueryAndResults(orm, "errorQuery", func() ([]string, error) { return nil, fmt.Errorf("error") })
	require.Error(t, err)

	require.Equal(t, 1, counterFromHistogramByLabels(t, orm.queryDuration, "420", "errorQuery", "read"))
	require.Equal(t, 0, counterFromGaugeByLabels(orm.datasetSize, "420", "errorQuery", "read"))
}

func TestMetricsAreProperlyPopulatedForWrites(t *testing.T) {
	orm := createObservedORM(t, "420")
	require.NoError(t, withObservedExec(orm, "execQuery", create, func() error { return nil }))
	require.Error(t, withObservedExec(orm, "execQuery", create, func() error { return fmt.Errorf("error") }))

	require.Equal(t, 2, counterFromHistogramByLabels(t, orm.queryDuration, "420", "execQuery", "create"))
}

func TestCountersAreProperlyPopulatedForWrites(t *testing.T) {
	ctx := tests.Context(t)
	orm := createObservedORM(t, "420")
	logs := generateRandomLogs(t, 100, 20)

	// First insert 10 logs
	require.NoError(t, orm.InsertLogs(ctx, logs[:10]))
	assert.Equal(t, float64(10), testutil.ToFloat64(orm.logsInserted.WithLabelValues("420")))

	// Insert 5 more logs
	require.NoError(t, orm.InsertLogs(ctx, logs[10:15]))
	assert.Equal(t, float64(15), testutil.ToFloat64(orm.logsInserted.WithLabelValues("420")))
	assert.Equal(t, float64(1), testutil.ToFloat64(orm.blocksInserted.WithLabelValues("420")))

	// Insert 5 more logs
	require.NoError(t, orm.InsertLogs(ctx, logs[15:]))
	assert.Equal(t, float64(20), testutil.ToFloat64(orm.logsInserted.WithLabelValues("420")))
	assert.Equal(t, float64(2), testutil.ToFloat64(orm.blocksInserted.WithLabelValues("420")))
}

func generateRandomLogs(t *testing.T, filterID int64, count int) []Log {
	logs := make([]Log, count)
	for i := range logs {
		logs[i] = newRandomLog(t, filterID, chainID, "My Event")
	}
	return logs
}

func createObservedORM(t *testing.T, chainId string) *ObservedORM {
	lggr := logger.Test(t)
	db := sqltest.NewDB(t, sqltest.TestURL(t))
	return NewObservedORM(chainId, db, lggr)
}

func resetMetrics(lp ObservedORM) {
	lp.queryDuration.Reset()
	lp.datasetSize.Reset()
	lp.logsInserted.Reset()
	lp.blocksInserted.Reset()
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
	pb := &io_prometheus_client.Metric{}
	err = metric.Write(pb)
	require.NoError(t, err)

	return int(pb.GetHistogram().GetSampleCount())
}
