package logpoller

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
)

type queryType string

const (
	create queryType = "create"
	read   queryType = "read"
	del    queryType = "delete"
)

var (
	sqlLatencyBuckets = []float64{
		float64(1 * time.Millisecond),
		float64(5 * time.Millisecond),
		float64(10 * time.Millisecond),
		float64(20 * time.Millisecond),
		float64(30 * time.Millisecond),
		float64(40 * time.Millisecond),
		float64(50 * time.Millisecond),
		float64(60 * time.Millisecond),
		float64(70 * time.Millisecond),
		float64(80 * time.Millisecond),
		float64(90 * time.Millisecond),
		float64(100 * time.Millisecond),
		float64(200 * time.Millisecond),
		float64(300 * time.Millisecond),
		float64(400 * time.Millisecond),
		float64(500 * time.Millisecond),
		float64(750 * time.Millisecond),
		float64(1 * time.Second),
		float64(2 * time.Second),
		float64(5 * time.Second),
	}
	lpQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "solana_log_poller_query_duration",
		Help:    "Measures duration of Log Poller's queries fetching logs",
		Buckets: sqlLatencyBuckets,
	}, []string{"chainID", "query", "type"})
	lpQueryDataSets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "solana_log_poller_query_dataset_size",
		Help: "Measures size of the datasets returned by Log Poller's queries",
	}, []string{"chainID", "query", "type"})
	lpLogsInserted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_log_poller_logs_inserted",
		Help: "Counter to track number of logs inserted by Log Poller",
	}, []string{"chainID"})
)

// ObservedORM is a decorator layer for ORM used by LogPoller, responsible for pushing Prometheus metrics reporting duration and size of result set for the queries.
// It doesn't change internal logic, because all calls are delegated to the origin ORM
type ObservedORM struct {
	ORM
	queryDuration *prometheus.HistogramVec
	datasetSize   *prometheus.GaugeVec
	logsInserted  *prometheus.CounterVec
	chainID       string
}

var _ ORM = &ObservedORM{}

// NewObservedORM creates an observed version of log poller's ORM created by NewORM
// Please see ObservedLogPoller for more details on how latencies are measured
func NewObservedORM(chainID string, ds sqlutil.DataSource, lggr logger.Logger) *ObservedORM {
	return &ObservedORM{
		ORM:           NewORM(chainID, ds, lggr),
		queryDuration: lpQueryDuration,
		datasetSize:   lpQueryDataSets,
		logsInserted:  lpLogsInserted,
		chainID:       chainID,
	}
}

func (o *ObservedORM) InsertLogs(ctx context.Context, logs []Log) error {
	err := withObservedExec(o, "InsertLogs", create, func() error {
		return o.ORM.InsertLogs(ctx, logs)
	})
	trackInsertedLogs(o, logs, err)
	return err
}

func (o *ObservedORM) InsertFilter(ctx context.Context, filter Filter) (id int64, err error) {
	return id, withObservedExec(o, "InsertFilter", create, func() (err error) {
		id, err = o.ORM.InsertFilter(ctx, filter)
		return err
	})
}

func (o *ObservedORM) SelectFilters(ctx context.Context) ([]Filter, error) {
	return withObservedQuery(o, "SelectFilters", func() ([]Filter, error) {
		return o.ORM.SelectFilters(ctx)
	})
}

func (o *ObservedORM) DeleteFilters(ctx context.Context, filters map[int64]Filter) error {
	return withObservedExec(o, "DeleteFilters", del, func() error {
		return o.ORM.DeleteFilters(ctx, filters)
	})
}

func (o *ObservedORM) MarkFilterDeleted(ctx context.Context, id int64) error {
	return withObservedExec(o, "MarkFilterDeleted", create, func() error {
		return o.ORM.MarkFilterDeleted(ctx, id)
	})
}

func (o *ObservedORM) MarkFilterBackfilled(ctx context.Context, id int64) error {
	return withObservedExec(o, "MarkFilterBackfilled", create, func() error {
		return o.ORM.MarkFilterBackfilled(ctx, id)
	})
}

func (o *ObservedORM) SelectSeqNums(ctx context.Context) (map[int64]int64, error) {
	return withObservedQuery(o, "SelectSeqNums", func() (map[int64]int64, error) {
		return o.ORM.SelectSeqNums(ctx)
	})
}

func (o *ObservedORM) FilteredLogs(ctx context.Context, filter []query.Expression, limitAndSort query.LimitAndSort, queryName string) ([]Log, error) {
	return withObservedQueryAndResults(o, queryName, func() ([]Log, error) {
		return o.ORM.FilteredLogs(ctx, filter, limitAndSort, queryName)
	})
}

func (o *ObservedORM) GetLatestBlock(ctx context.Context) (int64, error) {
	return withObservedQuery(o, "GetLatestBlack", func() (int64, error) {
		return o.ORM.GetLatestBlock(ctx)
	})
}

func (o *ObservedORM) PruneLogsForFilter(ctx context.Context, filter Filter) (int64, error) {
	return withObservedExecAndRowsAffected(o, "PruneLogsForFilter", del, func() (int64, error) {
		return o.ORM.PruneLogsForFilter(ctx, filter)
	})
}

func withObservedQueryAndResults[T any](o *ObservedORM, queryName string, query func() ([]T, error)) ([]T, error) {
	results, err := withObservedQuery(o, queryName, query)
	if err == nil {
		o.datasetSize.
			WithLabelValues(o.chainID, queryName, string(read)).
			Set(float64(len(results)))
	}
	return results, err
}

func withObservedQuery[T any](o *ObservedORM, queryName string, query func() (T, error)) (T, error) {
	queryStarted := time.Now()
	defer func() {
		o.queryDuration.
			WithLabelValues(o.chainID, queryName, string(read)).
			Observe(float64(time.Since(queryStarted)))
	}()
	return query()
}

func withObservedExec(o *ObservedORM, query string, queryType queryType, exec func() error) error {
	queryStarted := time.Now()
	defer func() {
		o.queryDuration.
			WithLabelValues(o.chainID, query, string(queryType)).
			Observe(float64(time.Since(queryStarted)))
	}()
	return exec()
}

func withObservedExecAndRowsAffected(o *ObservedORM, queryName string, queryType queryType, exec func() (int64, error)) (int64, error) {
	queryStarted := time.Now()
	rowsAffected, err := exec()
	o.queryDuration.
		WithLabelValues(o.chainID, queryName, string(queryType)).
		Observe(float64(time.Since(queryStarted)))

	if err == nil {
		o.datasetSize.
			WithLabelValues(o.chainID, queryName, string(queryType)).
			Set(float64(rowsAffected))
	}

	return rowsAffected, err
}

func trackInsertedLogs(o *ObservedORM, logs []Log, err error) {
	if err != nil {
		return
	}
	o.logsInserted.
		WithLabelValues(o.chainID).
		Add(float64(len(logs)))
}
