package monitor

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
)

// These tests mutate the global beholder client: no t.Parallel in this file.
// Each test uses t.Name() as the chainID so the package-level Prometheus
// gauges never share a series between tests.

// setupHistoryReader installs a ManualReader-backed beholder meter before monitor construction.
func setupHistoryReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	prevClient := beholder.GetClient()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	client := beholder.NewNoopClient()
	client.Meter = provider.Meter("beholder")
	client.MeterProvider = provider
	beholder.SetClient(client)
	t.Cleanup(func() { beholder.SetClient(prevClient) })
	return reader
}

func newTestHistoryMonitor(t *testing.T, period time.Duration, nodes ...NodeHistory) (*historyMonitor, *sdkmetric.ManualReader) {
	t.Helper()
	reader := setupHistoryReader(t)
	svc, err := NewHistoryMonitor(logger.Test(t), t.Name(), period, nodes)
	require.NoError(t, err)
	return svc.(*historyMonitor), reader
}

// mustGaugeValue returns the OTel gauge value for one chainID and nodeName.
func mustGaugeValue(t *testing.T, reader *sdkmetric.ManualReader, name, chainID, nodeName string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "metric %s must be an int64 gauge", name)
			for _, dp := range g.DataPoints {
				gotChain, okC := dp.Attributes.Value("chainID")
				gotNode, okN := dp.Attributes.Value("nodeName")
				if okC && okN && gotChain.AsString() == chainID && gotNode.AsString() == nodeName {
					return dp.Value
				}
			}
			t.Fatalf("metric %s has no datapoint for chainID=%s nodeName=%s", name, chainID, nodeName)
		}
	}
	t.Fatalf("metric %s not found in beholder export", name)
	return 0
}

func requireNoPromSeries(t *testing.T, name, chainID string) {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "chainID" && label.GetValue() == chainID {
					t.Fatalf("metric %s still has a series for chainID=%s", name, chainID)
				}
			}
		}
	}
}

func requireNoOTelSeries(t *testing.T, reader *sdkmetric.ManualReader, name, chainID string) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok)
			for _, dp := range g.DataPoints {
				gotChain, ok := dp.Attributes.Value("chainID")
				if ok && gotChain.AsString() == chainID {
					t.Fatalf("metric %s still has a series for chainID=%s", name, chainID)
				}
			}
		}
	}
}

func TestHistoryMonitorRejectsNonpositivePeriod(t *testing.T) {
	for _, period := range []time.Duration{0, -time.Second} {
		svc, err := NewHistoryMonitor(logger.Test(t), t.Name(), period, nil)
		require.Nil(t, svc)
		require.ErrorContains(t, err, "history monitor period must be positive")
	}
}

func TestHistoryMonitorRecordsPerNodeValues(t *testing.T) {
	clientA := mocks.NewReaderWriter(t)
	clientB := mocks.NewReaderWriter(t)
	m, reader := newTestHistoryMonitor(t, time.Hour,
		NodeHistory{Name: "a", Client: clientA},
		NodeHistory{Name: "b", Client: clientB})
	require.NoError(t, m.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, m.Close()) })

	clientA.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(100), nil).Once()
	clientB.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(200), nil).Once()

	m.measureNode(t.Context(), m.nodes[0])
	m.measureNode(t.Context(), m.nodes[1])

	require.Equal(t, float64(100), testutil.ToFloat64(promRPCFirstAvailableSlot.WithLabelValues(t.Name(), "a")))
	require.Equal(t, float64(200), testutil.ToFloat64(promRPCFirstAvailableSlot.WithLabelValues(t.Name(), "b")))
	require.Equal(t, int64(100), mustGaugeValue(t, reader, rpcFirstAvailableSlotName, t.Name(), "a"))
	require.Equal(t, int64(200), mustGaugeValue(t, reader, rpcFirstAvailableSlotName, t.Name(), "b"))
	require.Greater(t, mustGaugeValue(t, reader, rpcHistoryLastSuccessName, t.Name(), "a"), int64(0))
	require.Greater(t, mustGaugeValue(t, reader, rpcHistoryLastSuccessName, t.Name(), "b"), int64(0))
}

func TestHistoryMonitorAcceptsSlotZero(t *testing.T) {
	client := mocks.NewReaderWriter(t)
	m, _ := newTestHistoryMonitor(t, 10*time.Millisecond, NodeHistory{Name: "a", Client: client})

	client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(0), nil).Once()

	m.measureNode(t.Context(), m.nodes[0])

	require.Equal(t, float64(0), testutil.ToFloat64(promRPCFirstAvailableSlot.WithLabelValues(t.Name(), "a")))
	require.Greater(t, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "a")), float64(0))
}

func TestHistoryMonitorOverflow(t *testing.T) {
	client := mocks.NewReaderWriter(t)
	m, _ := newTestHistoryMonitor(t, 10*time.Millisecond, NodeHistory{Name: "a", Client: client})

	client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(math.MaxInt64)+1, nil).Once()

	m.measureNode(t.Context(), m.nodes[0])

	require.Zero(t, testutil.ToFloat64(promRPCFirstAvailableSlot.WithLabelValues(t.Name(), "a")))
	require.Zero(t, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "a")))
}

func TestHistoryMonitorFailureKeepsGauges(t *testing.T) {
	t.Run("rpc error", func(t *testing.T) {
		client := mocks.NewReaderWriter(t)
		m, _ := newTestHistoryMonitor(t, 10*time.Millisecond, NodeHistory{Name: "a", Client: client})

		client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(0), errors.New("boom")).Once()
		m.measureNode(t.Context(), m.nodes[0])

		require.Zero(t, testutil.ToFloat64(promRPCFirstAvailableSlot.WithLabelValues(t.Name(), "a")))
		require.Zero(t, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "a")))
	})

	t.Run("failure after success leaves values unchanged", func(t *testing.T) {
		client := mocks.NewReaderWriter(t)
		m, _ := newTestHistoryMonitor(t, 10*time.Millisecond, NodeHistory{Name: "a", Client: client})

		client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(42), nil).Once()
		m.measureNode(t.Context(), m.nodes[0])
		lastSuccess := testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "a"))
		require.Greater(t, lastSuccess, float64(0))

		client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(0), errors.New("boom")).Once()
		m.measureNode(t.Context(), m.nodes[0])
		require.Equal(t, float64(42), testutil.ToFloat64(promRPCFirstAvailableSlot.WithLabelValues(t.Name(), "a")))
		require.Equal(t, lastSuccess, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "a")))
	})
}

func TestHistoryMonitorConstructorHasNoMetricSideEffects(t *testing.T) {
	client := mocks.NewReaderWriter(t)
	_, reader := newTestHistoryMonitor(t, time.Hour, NodeHistory{Name: "a", Client: client})

	requireNoPromSeries(t, rpcFirstAvailableSlotName, t.Name())
	requireNoPromSeries(t, rpcHistoryLastSuccessName, t.Name())
	requireNoOTelSeries(t, reader, rpcFirstAvailableSlotName, t.Name())
	requireNoOTelSeries(t, reader, rpcHistoryLastSuccessName, t.Name())
}

func TestHistoryMonitorSeedsLastSuccessAtZero(t *testing.T) {
	clientA := mocks.NewReaderWriter(t)
	clientB := mocks.NewReaderWriter(t)
	m, reader := newTestHistoryMonitor(t, time.Hour,
		NodeHistory{Name: "a", Client: clientA},
		NodeHistory{Name: "b", Client: clientB})
	require.NoError(t, m.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, m.Close()) })

	require.Zero(t, mustGaugeValue(t, reader, rpcHistoryLastSuccessName, t.Name(), "a"))
	require.Zero(t, mustGaugeValue(t, reader, rpcHistoryLastSuccessName, t.Name(), "b"))
	require.Zero(t, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "a")))
	require.Zero(t, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "b")))
}

func TestHistoryMonitorNoRequestInsideStart(t *testing.T) {
	client := mocks.NewReaderWriter(t)
	var calls atomic.Int64
	client.EXPECT().GetFirstAvailableBlock(mock.Anything).RunAndReturn(func(context.Context) (uint64, error) {
		calls.Add(1)
		return 0, nil
	}).Maybe()

	m, _ := newTestHistoryMonitor(t, 300*time.Millisecond, NodeHistory{Name: "a", Client: client})
	require.NoError(t, m.Start(t.Context()))

	time.Sleep(100 * time.Millisecond)
	require.Zero(t, calls.Load())

	require.NoError(t, m.Close())
}

func TestHistoryMonitorFailingNodeDoesNotStopOthers(t *testing.T) {
	bad := mocks.NewReaderWriter(t)
	good := mocks.NewReaderWriter(t)
	bad.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(0), errors.New("boom")).Maybe()
	good.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(7), nil).Maybe()

	m, _ := newTestHistoryMonitor(t, 10*time.Millisecond,
		NodeHistory{Name: "bad", Client: bad},
		NodeHistory{Name: "good", Client: good})
	require.NoError(t, m.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, m.Close()) })

	require.Eventually(t, func() bool {
		return testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "good")) > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.Zero(t, testutil.ToFloat64(promRPCHistoryLastSuccess.WithLabelValues(t.Name(), "bad")))
}

func TestHistoryMonitorCloseDeletesSeries(t *testing.T) {
	client := mocks.NewReaderWriter(t)
	client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(uint64(1), nil).Once()
	m, reader := newTestHistoryMonitor(t, time.Hour, NodeHistory{Name: "a", Client: client})
	require.NoError(t, m.Start(t.Context()))
	m.measureNode(t.Context(), m.nodes[0])

	require.NoError(t, m.Close())

	requireNoPromSeries(t, rpcFirstAvailableSlotName, t.Name())
	requireNoPromSeries(t, rpcHistoryLastSuccessName, t.Name())
	requireNoOTelSeries(t, reader, rpcFirstAvailableSlotName, t.Name())
	requireNoOTelSeries(t, reader, rpcHistoryLastSuccessName, t.Name())
}

func TestHistoryMonitorCloseCancelsMeasurements(t *testing.T) {
	client := mocks.NewReaderWriter(t)
	var calls atomic.Int64
	client.EXPECT().GetFirstAvailableBlock(mock.Anything).RunAndReturn(func(ctx context.Context) (uint64, error) {
		calls.Add(1)
		<-ctx.Done() // block until the monitor cancels the measurement
		return 0, ctx.Err()
	}).Maybe()

	m, _ := newTestHistoryMonitor(t, 10*time.Millisecond, NodeHistory{Name: "a", Client: client})
	require.NoError(t, m.Start(t.Context()))

	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, m.Close())
	afterClose := calls.Load()
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, afterClose, calls.Load())

	requireNoPromSeries(t, rpcFirstAvailableSlotName, t.Name())
	requireNoPromSeries(t, rpcHistoryLastSuccessName, t.Name())
}
