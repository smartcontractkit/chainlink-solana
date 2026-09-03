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

var promLpLastProcessedSlot = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "solana_log_poller_last_processed_slot",
	Help: "Last processed slot by log poller",
}, []string{"chainID"})

var promLpBlocksSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "solana_log_poller_blocks_skipped",
	Help: "Number of blocks skipped due to max retry exhaustion",
}, []string{"chainID"})

var promLpEventsSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "solana_log_poller_events_skipped",
	Help: "Number of events skipped due to malformed event data",
}, []string{"chainID"})

var promLpTxsUnsupportedVersion = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "solana_log_poller_txs_unsupported_version",
	Help: "Number of transactions skipped because their version cannot be decoded. Events in these transactions are not indexed",
}, []string{"chainID"})

type solLpMetrics struct {
	metrics.Labeler
	chainID string

	// transactions
	txsTruncated       outcomeDependantMetric
	txsLogParsingError outcomeDependantMetric
	lastProcessedSlot  metric.Int64Gauge
	blocksSkipped      metric.Int64Counter
	eventsSkipped      metric.Int64Counter
	txsUnsupportedVer  metric.Int64Counter
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

	lastProcessedSlot, err := meter.Int64Gauge("solana_log_poller_last_processed_slot")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana_log_poller_last_processed_slot: %w", err)
	}

	blocksSkipped, err := meter.Int64Counter("solana_log_poller_blocks_skipped")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana_log_poller_blocks_skipped: %w", err)
	}

	eventsSkipped, err := meter.Int64Counter("solana_log_poller_events_skipped")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana_log_poller_events_skipped: %w", err)
	}

	txsUnsupportedVer, err := meter.Int64Counter("solana_log_poller_txs_unsupported_version")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana_log_poller_txs_unsupported_version: %w", err)
	}

	return &solLpMetrics{
		chainID: chainID,
		Labeler: metrics.NewLabeler().With("chainID", chainID),

		txsTruncated:       *truncatedTxs,
		txsLogParsingError: *txLogParsingError,
		lastProcessedSlot:  lastProcessedSlot,
		blocksSkipped:      blocksSkipped,
		eventsSkipped:      eventsSkipped,
		txsUnsupportedVer:  txsUnsupportedVer,
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

func (m *solLpMetrics) SetLatestProcessedSlot(ctx context.Context, slot int64) {
	promLpLastProcessedSlot.WithLabelValues(m.chainID).Set(float64(slot))
	m.lastProcessedSlot.Record(ctx, slot, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solLpMetrics) IncrementBlocksSkipped(ctx context.Context) {
	promLpBlocksSkipped.WithLabelValues(m.chainID).Inc()
	m.blocksSkipped.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solLpMetrics) IncrementEventsSkipped(ctx context.Context) {
	promLpEventsSkipped.WithLabelValues(m.chainID).Inc()
	m.eventsSkipped.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

// IncrementTxsUnsupportedVersion counts transactions dropped because their version is
// newer than this client can decode. Any events they contain are not indexed.
func (m *solLpMetrics) IncrementTxsUnsupportedVersion(ctx context.Context) {
	promLpTxsUnsupportedVersion.WithLabelValues(m.chainID).Inc()
	m.txsUnsupportedVer.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
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
