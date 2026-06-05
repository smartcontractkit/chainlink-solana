package monitor

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-framework/metrics"
)

func TestUpdateBalanceMetrics(t *testing.T) {
	// Mutates the global beholder client, cannot run in parallel with other beholder tests.
	key := solana.PublicKey{}
	balance := uint64(1_000_000_000)
	want := float64(balance) / float64(solana.LAMPORTS_PER_SOL)
	chainID := "test-chain"

	balanceMetrics, reader := setupTestBalanceMetrics(t, chainID)
	monitor := balanceMonitor{
		chainID:        chainID,
		balanceMetrics: balanceMetrics,
	}
	monitor.updateProm(t.Context(), key, balance)

	t.Run("node_balance_prometheus", func(t *testing.T) {
		got := testutil.ToFloat64(metrics.NodeBalance.WithLabelValues(key.String(), chainID, metrics.Solana))
		require.InEpsilon(t, want, got, 0.001)
	})

	t.Run("node_balance_beholder", func(t *testing.T) {
		got := mustNodeBalanceGauge(t, reader, key.String(), chainID, metrics.Solana)
		require.InEpsilon(t, want, got, 0.001)
	})

	t.Run("solana_balance_deprecated", func(t *testing.T) {
		got := testutil.ToFloat64(promSolanaBalance.WithLabelValues(key.String(), chainID, "solana", "SOL"))
		require.InEpsilon(t, want, got, 0.001)
	})
}

// setupTestBalanceMetrics installs a ManualReader-backed beholder meter before
// constructing GenericBalanceMetrics, since the OTel gauge is registered at init time.
func setupTestBalanceMetrics(t *testing.T, chainID string) (metrics.GenericBalanceMetrics, *sdkmetric.ManualReader) {
	t.Helper()

	prevClient := beholder.GetClient()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	client := beholder.NewNoopClient()
	client.Meter = provider.Meter("beholder")
	client.MeterProvider = provider
	beholder.SetClient(client)

	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		beholder.SetClient(prevClient)
	})

	balanceMetrics, err := metrics.NewGenericBalanceMetrics(metrics.Solana, chainID)
	require.NoError(t, err)
	return balanceMetrics, reader
}

func mustNodeBalanceGauge(t *testing.T, reader *sdkmetric.ManualReader, account, chainID, chainFamily string) float64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	wantAttrs := map[string]string{
		"account":     account,
		"chainID":     chainID,
		"chainFamily": chainFamily,
	}

	var matches []float64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "node_balance" {
				continue
			}
			gauge, ok := m.Data.(metricdata.Gauge[float64])
			require.True(t, ok, "node_balance should be a float64 gauge")
			for _, dp := range gauge.DataPoints {
				if attrsMatch(dp.Attributes, wantAttrs) {
					matches = append(matches, dp.Value)
				}
			}
		}
	}
	require.Len(t, matches, 1, "expected exactly one node_balance datapoint for account=%s chainID=%s chainFamily=%s", account, chainID, chainFamily)
	return matches[0]
}

func attrsMatch(set attribute.Set, want map[string]string) bool {
	for key, value := range want {
		if attrString(set, key) != value {
			return false
		}
	}
	return true
}

func attrString(set attribute.Set, key string) string {
	for _, kv := range set.ToSlice() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}
