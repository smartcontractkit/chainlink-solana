package txm

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

var (
	// successful transactions
	promSolTxmSuccessTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_success",
		Help: "Number of transactions that are included and successfully executed on chain",
	}, []string{"chainID"})
	promSolTxmFinalizedTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_finalized",
		Help: "Number of transactions that are finalized on chain",
	}, []string{"chainID"})

	// inflight transactions
	promSolTxmPendingTxs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "solana_txm_tx_pending",
		Help: "Number of transactions that are pending confirmation",
	}, []string{"chainID"})

	// error cases
	promSolTxmErrorTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error",
		Help: "Number of transactions that have errored across all cases",
	}, []string{"chainID"})
	promSolTxmRevertTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error_revert",
		Help: "Number of transactions that are included and failed onchain",
	}, []string{"chainID"})
	promSolTxmRejectTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error_reject",
		Help: "Number of transactions that the RPC immediately rejected",
	}, []string{"chainID"})
	promSolTxmDropTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error_drop",
		Help: "Number of transactions that timed out during confirmation. Note: tx is likely dropped from the chain, but may still be included.",
	}, []string{"chainID"})
	promSolTxmSimRevertTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error_sim_revert",
		Help: "Number of transactions that reverted during simulation. Note: tx may still be included onchain",
	}, []string{"chainID"})
	promSolTxmSimOtherTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error_sim_other",
		Help: "Number of transactions that failed simulation with an unrecognized error. Note: tx may still be included onchain",
	}, []string{"chainID"})
	promSolTxmDependencyFailTxs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_tx_error_dependency",
		Help: "Number of transactions that failed due to a dependency tx failing.",
	}, []string{"chainID"})

	// transaction fees
	promSolTxmFeeBumps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "solana_txm_fee_bumps",
		Help: "Number of fee bumps made to get transactions included on-chain",
	}, []string{"chainID"})
)

type solTxmMetrics struct {
	metrics.Labeler
	chainID string

	// successful transactions
	successTxs   metric.Int64Counter
	finalizedTxs metric.Int64Counter

	// inflight transactions
	pendingTxs metric.Int64Gauge

	// error cases
	errorTxs          metric.Int64Counter
	revertTxs         metric.Int64Counter
	rejectTxs         metric.Int64Counter
	dropTxs           metric.Int64Counter
	simRevertTxs      metric.Int64Counter
	simOtherTxs       metric.Int64Counter
	dependencyFailTxs metric.Int64Counter

	// transaction fees
	feeBumps metric.Int64Counter
}

func newSolTxmMetrics(chainID string) (*solTxmMetrics, error) {
	m := beholder.GetMeter()
	var err error

	successTxs, err := m.Int64Counter("solana_txm_tx_success")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana success txs: %w", err)
	}

	finalizedTxs, err := m.Int64Counter("solana_txm_tx_finalized")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana finalized txs: %w", err)
	}

	pendingTxs, err := m.Int64Gauge("solana_txm_tx_pending")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana pending txs: %w", err)
	}

	errorTxs, err := m.Int64Counter("solana_txm_tx_error")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana error txs: %w", err)
	}

	revertTxs, err := m.Int64Counter("solana_txm_tx_error_revert")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana revert txs: %w", err)
	}

	rejectTxs, err := m.Int64Counter("solana_txm_tx_error_reject")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana reject txs: %w", err)
	}

	dropTxs, err := m.Int64Counter("solana_txm_tx_error_drop")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana drop txs: %w", err)
	}

	simRevertTxs, err := m.Int64Counter("solana_txm_tx_error_sim_revert")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana sim revert txs: %w", err)
	}

	simOtherTxs, err := m.Int64Counter("solana_txm_tx_error_sim_other")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana sim other txs: %w", err)
	}

	dependencyFailTxs, err := m.Int64Counter("solana_txm_tx_error_dependency")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana dependency fail txs: %w", err)
	}

	feeBumps, err := m.Int64Counter("solana_txm_fee_bumps")
	if err != nil {
		return nil, fmt.Errorf("failed to register solana fee bumps counter: %w", err)
	}

	return &solTxmMetrics{
		chainID: chainID,
		Labeler: metrics.NewLabeler().With("chainID", chainID),

		successTxs:        successTxs,
		finalizedTxs:      finalizedTxs,
		pendingTxs:        pendingTxs,
		errorTxs:          errorTxs,
		revertTxs:         revertTxs,
		rejectTxs:         rejectTxs,
		dropTxs:           dropTxs,
		simRevertTxs:      simRevertTxs,
		simOtherTxs:       simOtherTxs,
		dependencyFailTxs: dependencyFailTxs,
		feeBumps:          feeBumps,
	}, nil
}

func (m *solTxmMetrics) GetOtelAttributes() []attribute.KeyValue {
	return beholder.OtelAttributes(m.Labels).AsStringAttributes()
}

func (m *solTxmMetrics) IncrementSuccessTxs(ctx context.Context) {
	promSolTxmSuccessTxs.WithLabelValues(m.chainID).Add(1)
	m.successTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementFinalizedTxs(ctx context.Context) {
	promSolTxmFinalizedTxs.WithLabelValues(m.chainID).Add(1)
	m.finalizedTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) SetPendingTxs(ctx context.Context, count int) {
	promSolTxmPendingTxs.WithLabelValues(m.chainID).Set(float64(count))
	m.pendingTxs.Record(ctx, int64(count), metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementErrorTxs(ctx context.Context) {
	promSolTxmErrorTxs.WithLabelValues(m.chainID).Add(1)
	m.errorTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementRevertTxs(ctx context.Context) {
	promSolTxmRevertTxs.WithLabelValues(m.chainID).Add(1)
	m.revertTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementRejectTxs(ctx context.Context) {
	promSolTxmRejectTxs.WithLabelValues(m.chainID).Add(1)
	m.rejectTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementDropTxs(ctx context.Context) {
	promSolTxmDropTxs.WithLabelValues(m.chainID).Add(1)
	m.dropTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementSimRevertTxs(ctx context.Context) {
	promSolTxmSimRevertTxs.WithLabelValues(m.chainID).Add(1)
	m.simRevertTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementSimOtherTxs(ctx context.Context) {
	promSolTxmSimOtherTxs.WithLabelValues(m.chainID).Add(1)
	m.simOtherTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementDependencyFailTxs(ctx context.Context) {
	promSolTxmDependencyFailTxs.WithLabelValues(m.chainID).Add(1)
	m.dependencyFailTxs.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}

func (m *solTxmMetrics) IncrementFeeBumps(ctx context.Context) {
	promSolTxmFeeBumps.WithLabelValues(m.chainID).Add(1)
	m.feeBumps.Add(ctx, 1, metric.WithAttributes(m.GetOtelAttributes()...))
}
