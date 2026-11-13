package monitor

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

const (
	rpcFirstAvailableSlotName = "solana_rpc_first_available_slot"
	rpcHistoryLastSuccessName = "solana_rpc_history_last_success_timestamp_seconds"
	historyRequestTimeout     = 10 * time.Second
)

var (
	promRPCFirstAvailableSlot = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: rpcFirstAvailableSlotName,
		Help: "Oldest available block slot reported by the RPC node",
	}, []string{"chainID", "nodeName"})

	promRPCHistoryLastSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: rpcHistoryLastSuccessName,
		Help: "Unix time of the last successful RPC history measurement",
	}, []string{"chainID", "nodeName"})
)

// HistoryClient returns the oldest available block of one node.
type HistoryClient interface {
	GetFirstAvailableBlock(ctx context.Context) (uint64, error)
}

// NodeHistory pairs a node name with its RPC client.
type NodeHistory struct {
	Name   string
	Client HistoryClient
}

type historyNode struct {
	name   string
	client HistoryClient
	attrs  []attribute.KeyValue
}

type historyValues struct {
	slot        int64
	lastSuccess int64
	measured    bool
}

type historyMonitor struct {
	services.Service
	engine       *services.Engine
	chainID      string
	registration metric.Registration

	slotGauge   metric.Int64ObservableGauge
	lastSuccess metric.Int64ObservableGauge

	mu     sync.RWMutex
	values map[string]historyValues
	nodes  []historyNode
}

// NewHistoryMonitor returns a service which measures the RPC history depth of
// each node once per period. The first measurement runs one period after Start.
// A failed measurement leaves the gauges unchanged.
func NewHistoryMonitor(lggr logger.Logger, chainID string, period time.Duration, nodes []NodeHistory) (services.Service, error) {
	if period <= 0 {
		return nil, fmt.Errorf("history monitor period must be positive: %s", period)
	}

	meter := beholder.GetMeter()

	slotGauge, err := meter.Int64ObservableGauge(rpcFirstAvailableSlotName)
	if err != nil {
		return nil, fmt.Errorf("failed to register %s: %w", rpcFirstAvailableSlotName, err)
	}

	lastSuccessGauge, err := meter.Int64ObservableGauge(rpcHistoryLastSuccessName)
	if err != nil {
		return nil, fmt.Errorf("failed to register %s: %w", rpcHistoryLastSuccessName, err)
	}

	m := &historyMonitor{
		chainID:     chainID,
		slotGauge:   slotGauge,
		lastSuccess: lastSuccessGauge,
		values:      make(map[string]historyValues, len(nodes)),
	}

	for _, n := range nodes {
		hn := historyNode{
			name:   n.Name,
			client: n.Client,
			attrs: []attribute.KeyValue{
				attribute.String("chainID", chainID),
				attribute.String("nodeName", n.Name),
			},
		}
		m.nodes = append(m.nodes, hn)
		m.values[n.Name] = historyValues{}
	}

	m.Service, m.engine = services.Config{
		Name: "HistoryMonitor",
		Start: func(context.Context) error {
			m.registration, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
				m.observe(observer)
				return nil
			}, slotGauge, lastSuccessGauge)
			if err != nil {
				return fmt.Errorf("register history monitor callback: %w", err)
			}

			for _, n := range m.nodes {
				promRPCHistoryLastSuccess.WithLabelValues(chainID, n.name).Set(0)
				ticker := services.TickerConfig{Initial: period, JitterPct: services.DefaultJitter}.NewTicker(period)
				m.engine.GoTick(ticker, func(ctx context.Context) {
					m.measureNode(ctx, n)
				})
			}
			return nil
		},
		Close: func() error {
			for _, n := range m.nodes {
				promRPCFirstAvailableSlot.DeleteLabelValues(m.chainID, n.name)
				promRPCHistoryLastSuccess.DeleteLabelValues(m.chainID, n.name)
			}
			if err := m.registration.Unregister(); err != nil {
				return fmt.Errorf("unregister history monitor callback: %w", err)
			}
			return nil
		},
	}.NewServiceEngine(lggr)

	return m, nil
}

// observe exports the current values and omits the slot until a measurement succeeds.
func (m *historyMonitor) observe(observer metric.Observer) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, n := range m.nodes {
		v := m.values[n.name]
		attrs := metric.WithAttributes(n.attrs...)
		if v.measured {
			observer.ObserveInt64(m.slotGauge, v.slot, attrs)
		}
		observer.ObserveInt64(m.lastSuccess, v.lastSuccess, attrs)
	}
}

// measureNode records the oldest available slot of one node and the success
// time. A failure leaves both gauges unchanged.
func (m *historyMonitor) measureNode(ctx context.Context, n historyNode) {
	ctx, cancel := context.WithTimeout(ctx, historyRequestTimeout)
	defer cancel()

	oldestSlot, err := n.client.GetFirstAvailableBlock(ctx)
	if err != nil {
		m.engine.Warnw("RPC history measurement failed", "nodeName", n.name, "error", err)
		return
	}
	if oldestSlot > math.MaxInt64 {
		m.engine.Warnw("RPC history measurement failed", "nodeName", n.name,
			"error", fmt.Sprintf("first available slot %d overflows int64", oldestSlot))
		return
	}

	now := time.Now().Unix()
	m.mu.Lock()
	m.values[n.name] = historyValues{slot: int64(oldestSlot), lastSuccess: now, measured: true}
	m.mu.Unlock()
	promRPCFirstAvailableSlot.WithLabelValues(m.chainID, n.name).Set(float64(oldestSlot))
	promRPCHistoryLastSuccess.WithLabelValues(m.chainID, n.name).Set(float64(now))
}
