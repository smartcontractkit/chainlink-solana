package logpoller

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-framework/metrics"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// ObservedORM is a decorator layer for ORM used by LogPoller, responsible for pushing Prometheus metrics reporting duration and size of result set for the queries.
// It doesn't change internal logic, because all calls are delegated to the origin ORM
type ObservedORM struct {
	ORM
	metrics       metrics.GenericLogPollerMetrics
	queryDuration *prometheus.HistogramVec
	datasetSize   *prometheus.GaugeVec
	logsInserted  *prometheus.CounterVec
	chainID       string
}

var _ ORM = &ObservedORM{}

const timeout = 10 * time.Second

// NewObservedORM creates an observed version of log poller's ORM created by NewORM
// Please see ObservedLogPoller for more details on how latencies are measured
func NewObservedORM(chainID string, chainFamily string, ds sqlutil.DataSource, lggr logger.Logger) (*ObservedORM, error) {
	lpMetrics, err := metrics.NewGenericLogPollerMetrics(chainID, chainFamily)
	if err != nil {
		return nil, err
	}

	return &ObservedORM{
		ORM:           NewORM(chainID, ds, lggr),
		metrics:       lpMetrics,
		queryDuration: metrics.PromLpQueryDuration,
		datasetSize:   metrics.PromLpQueryDataSets,
		logsInserted:  metrics.PromLpLogsInserted,
		chainID:       chainID,
	}, nil
}

func (o *ObservedORM) InsertLogs(ctx context.Context, logs []types.Log) error {
	err := withObservedExec(o, "InsertLogs", metrics.Create, func() error {
		return o.ORM.InsertLogs(ctx, logs)
	})
	trackInsertedLogs(o, logs, err)
	return err
}

func (o *ObservedORM) InsertFilter(ctx context.Context, filter types.Filter) (id int64, err error) {
	return id, withObservedExec(o, "InsertFilter", metrics.Create, func() (err error) {
		id, err = o.ORM.InsertFilter(ctx, filter)
		return err
	})
}

func (o *ObservedORM) SelectFilters(ctx context.Context) ([]types.Filter, error) {
	return withObservedQuery(o, "SelectFilters", func() ([]types.Filter, error) {
		return o.ORM.SelectFilters(ctx)
	})
}

func (o *ObservedORM) DeleteFilters(ctx context.Context, filters map[int64]types.Filter) error {
	return withObservedExec(o, "DeleteFilters", metrics.Del, func() error {
		return o.ORM.DeleteFilters(ctx, filters)
	})
}

func (o *ObservedORM) MarkFilterDeleted(ctx context.Context, id int64) error {
	return withObservedExec(o, "MarkFilterDeleted", metrics.Create, func() error {
		return o.ORM.MarkFilterDeleted(ctx, id)
	})
}

func (o *ObservedORM) MarkFilterBackfilled(ctx context.Context, id int64) error {
	return withObservedExec(o, "MarkFilterBackfilled", metrics.Create, func() error {
		return o.ORM.MarkFilterBackfilled(ctx, id)
	})
}

func (o *ObservedORM) SelectSeqNums(ctx context.Context) (map[int64]int64, error) {
	return withObservedQuery(o, "SelectSeqNums", func() (map[int64]int64, error) {
		return o.ORM.SelectSeqNums(ctx)
	})
}

func (o *ObservedORM) FilteredLogs(ctx context.Context, filter []query.Expression, limitAndSort query.LimitAndSort, queryName string) ([]types.Log, error) {
	return withObservedQueryAndResults(o, queryName, func() ([]types.Log, error) {
		return o.ORM.FilteredLogs(ctx, filter, limitAndSort, queryName)
	})
}

func (o *ObservedORM) GetLatestBlock(ctx context.Context) (int64, error) {
	return withObservedQuery(o, "GetLatestBlack", func() (int64, error) {
		return o.ORM.GetLatestBlock(ctx)
	})
}

func (o *ObservedORM) PruneLogsForFilter(ctx context.Context, filter types.Filter) (int64, error) {
	return withObservedExecAndRowsAffected(o, "PruneLogsForFilter", metrics.Del, func() (int64, error) {
		return o.ORM.PruneLogsForFilter(ctx, filter)
	})
}

func withObservedQueryAndResults[T any](o *ObservedORM, queryName string, query func() ([]T, error)) ([]T, error) {
	results, err := withObservedQuery(o, queryName, query)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		o.metrics.RecordQueryDatasetSize(ctx, queryName, metrics.Read, int64(len(results)))
	}
	return results, err
}

func withObservedQuery[T any](o *ObservedORM, queryName string, query func() (T, error)) (T, error) {
	queryStarted := time.Now()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		o.metrics.RecordQueryDuration(ctx, queryName, metrics.Read, float64(time.Since(queryStarted)))
	}()
	return query()
}

func withObservedExec(o *ObservedORM, query string, queryType metrics.QueryType, exec func() error) error {
	queryStarted := time.Now()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		o.metrics.RecordQueryDuration(ctx, query, queryType, float64(time.Since(queryStarted)))
	}()
	return exec()
}

func withObservedExecAndRowsAffected(o *ObservedORM, queryName string, queryType metrics.QueryType, exec func() (int64, error)) (int64, error) {
	queryStarted := time.Now()
	rowsAffected, err := exec()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	o.metrics.RecordQueryDuration(ctx, queryName, queryType, float64(time.Since(queryStarted)))
	if err == nil {
		o.metrics.RecordQueryDatasetSize(ctx, queryName, queryType, rowsAffected)
	}

	return rowsAffected, err
}

func trackInsertedLogs(o *ObservedORM, logs []types.Log, err error) {
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	o.metrics.IncrementLogsInserted(ctx, int64(len(logs)))
}
