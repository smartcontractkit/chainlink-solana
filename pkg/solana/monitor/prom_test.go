package monitor

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-framework/metrics"
)

func TestPromSolBalance(t *testing.T) {
	key := solana.PublicKey{}
	balance := uint64(1_000_000_000)

	balanceMetrics, err := metrics.NewGenericBalanceMetrics(metrics.Solana, "test-chain")
	require.NoError(t, err)

	monitor := balanceMonitor{
		chainID:        "test-chain",
		balanceMetrics: balanceMetrics,
	}
	monitor.updateProm(t.Context(), key, balance)

	// happy path test
	promBalance := testutil.ToFloat64(metrics.NodeBalance.WithLabelValues(key.String(), monitor.chainID, metrics.Solana))
	assert.Equal(t, float64(balance)/float64(solana.LAMPORTS_PER_SOL), promBalance)
}
