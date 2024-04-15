package exporter

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestNodeBalances(t *testing.T) {
	ctx := utils.Context(t)
	lgr, logs := logger.TestObserved(t, zapcore.ErrorLevel)
	factory := NewNodeBalancesFactory(lgr, metrics.NewNodeBalances)

	chainConfig := testutils.GenerateChainConfig()
	feedConfig := testutils.GenerateFeedConfig()
	exporter, err := factory.NewExporter(commonMonitoring.ExporterParams{ChainConfig: chainConfig, FeedConfig: feedConfig, Nodes: []commonMonitoring.NodeConfig{}})
	require.NoError(t, err)

	// happy path
	exporter.Export(ctx, types.Balances{
		Addresses: map[string]solana.PublicKey{t.Name(): {}},
		Values:    map[string]uint64{t.Name(): 0},
	})

	exporter.Cleanup(ctx)

	// not balance type
	assert.NotPanics(t, func() { exporter.Export(ctx, 1) })

	// mismatch data
	exporter.Export(ctx, types.Balances{
		Addresses: map[string]solana.PublicKey{t.Name(): {}},
		Values:    map[string]uint64{},
	})
	tests.AssertLogEventually(t, logs, "mismatch addresses and balances")
}
