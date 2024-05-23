package exporter

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestNodeSuccess(t *testing.T) {
	zeroAddress := solana.PublicKey{}
	ctx := tests.Context(t)
	lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)
	m := mocks.NewNodeSuccess(t)
	m.On("Add", mock.Anything, mock.Anything).Once()
	m.On("Cleanup", mock.Anything).Once()

	factory := NewNodeSuccessFactory(lgr, m)

	chainConfig := testutils.GenerateChainConfig()
	feedConfig := testutils.GenerateFeedConfig()
	exporter, err := factory.NewExporter(commonMonitoring.ExporterParams{ChainConfig: chainConfig,
		FeedConfig: feedConfig,
		Nodes: []commonMonitoring.NodeConfig{
			config.SolanaNodeConfig{
				NodeAddress: []string{zeroAddress.String()}},
		}})
	require.NoError(t, err)

	// happy path - only one call (only 1 address is recognized)
	exporter.Export(ctx, []types.TxDetails{
		{Sender: zeroAddress},
		{Sender: solana.PublicKey{1}},
	})
	exporter.Cleanup(ctx)
	assert.Equal(t, 1, logs.FilterMessageSnippet("Sender does not match known operator").Len())

	// not txdetails type - no calls to mock
	assert.NotPanics(t, func() { exporter.Export(ctx, 1) })

	// zero txdetails - no calls to mock
	exporter.Export(ctx, []types.TxDetails{})
}
