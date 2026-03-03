package logpoller

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/metrics"
)

const txsTruncatedName = "solana_log_poller_txs_truncated"
const txsLogParsingErrorName = "solana_log_poller_txs_log_parsing_error"

var promSolLp = struct {
	txsTruncated       outcomeDependantProm
	txsLogParsingError outcomeDependantProm
}{
	txsTruncated:       newOutcomeDependantProm(txsTruncatedName, "Number of transactions that %s onchain but have truncated logs"),
	txsLogParsingError: newOutcomeDependantProm(txsLogParsingErrorName, "Number of transactions that %s onchain but had log parsing errors"),
}

type solLpMetrics struct {
	metrics.Labeler
	chainID string

	// transactions
	txsTruncated       outcomeDependantMetric
	txsLogParsingError outcomeDependantMetric
}

func NewSolLpMetrics(chainID string) (*solLpMetrics, error) {
	meter := beholder.GetMeter()

	truncatedTxs, err := newOutcomeDependantMetric(meter, txsTruncatedName)
	if err != nil {
		return nil, err
	}

	txLogParsingError, err := newOutcomeDependantMetric(meter, txsLogParsingErrorName)
	if err != nil {
		return nil, err
	}

	return &solLpMetrics{
		chainID: chainID,
		Labeler: metrics.NewLabeler().With("chainID", chainID),

		txsTruncated:       *truncatedTxs,
		txsLogParsingError: *txLogParsingError,
	}, nil
}

func (m *solLpMetrics) GetOtelAttributes() []attribute.KeyValue {
	return beholder.OtelAttributes(m.Labels).AsStringAttributes()
}

func (m *solLpMetrics) IncrementTruncatedTxs(ctx context.Context, txOutcome txOutcome) {
	m.incrementForOutcome(ctx, promSolLp.txsTruncated, m.txsTruncated, txOutcome)
}

func (m *solLpMetrics) IncrementTxsLogParsingError(ctx context.Context, txOutcome txOutcome) {
	m.incrementForOutcome(ctx, promSolLp.txsLogParsingError, m.txsLogParsingError, txOutcome)
}

func (m *solLpMetrics) incrementForOutcome(ctx context.Context, prom outcomeDependantProm, me outcomeDependantMetric, outcome txOutcome) {
	switch outcome {
	case txSucceeded:
		m.increment(ctx, prom.succeeded, me.succeeded)
	case txReverted:
		m.increment(ctx, prom.reverted, me.reverted)
	}
}

func (m *solLpMetrics) increment(ctx context.Context, prom *prometheus.CounterVec, me metric.Int64Counter) {
	prom.WithLabelValues(m.chainID).Add(1)
	me.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

type txOutcome string

const (
	txSucceeded txOutcome = "tx_succeeded"
	txReverted  txOutcome = "tx_reverted"
)

type outcomeDependantProm struct {
	succeeded *prometheus.CounterVec
	reverted  *prometheus.CounterVec
}

func newOutcomeDependantProm(name string, helpFormat string) outcomeDependantProm {
	return outcomeDependantProm{
		succeeded: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: succeeded(name),
			Help: fmt.Sprintf(helpFormat, "succeeded"),
		}, []string{"chainID"}),
		reverted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: reverted(name),
			Help: fmt.Sprintf(helpFormat, "reverted"),
		}, []string{"chainID"}),
	}
}

type outcomeDependantMetric struct {
	succeeded metric.Int64Counter
	reverted  metric.Int64Counter
}

func newOutcomeDependantMetric(meter metric.Meter, name string) (*outcomeDependantMetric, error) {
	succeededCounter, err := meter.Int64Counter(succeeded(name))
	if err != nil {
		return nil, fmt.Errorf("failed to register %s: %w", succeeded(name), err)
	}
	revertedCounter, err := meter.Int64Counter(reverted(name))
	if err != nil {
		return nil, fmt.Errorf("failed to register %s: %w", reverted(name), err)
	}

	return &outcomeDependantMetric{
		succeeded: succeededCounter,
		reverted:  revertedCounter,
	}, nil
}

func succeeded(name string) string {
	return name + "_succeeded"
}

func reverted(name string) string {
	return name + "_reverted"
}
