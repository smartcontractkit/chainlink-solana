package exporter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestFeedBalancePrometheusExporter(t *testing.T) {
	t.Parallel()

	t.Run("it should export balance updates then clean up", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		metrics := mocks.NewMetrics(t)
		factory := NewFeedBalancePrometheusExporterFactory(testutils.NewNullLogger(), metrics)

		chainConfig := testutils.GenerateChainConfig()
		feedConfig := testutils.GenerateFeedConfig()
		exporter, err := factory.NewExporter(commonMonitoring.ExporterParams{ChainConfig: chainConfig, FeedConfig: feedConfig, Nodes: []commonMonitoring.NodeConfig{}})
		require.NoError(t, err)

		balances := testutils.GenerateBalances()

		for _, accountName := range types.FeedBalanceAccountNames {
			metrics.On("SetBalance",
				balances.Values[accountName],
				accountName,
				balances.Addresses[accountName].String(),
				feedConfig.GetID(),
				chainConfig.GetChainID(),
				feedConfig.GetContractStatus(),
				feedConfig.GetContractType(),
				feedConfig.GetName(),
				feedConfig.GetPath(),
				chainConfig.GetNetworkID(),
				chainConfig.GetNetworkName(),
			)
		}
		exporter.Export(ctx, balances)

		for _, accountName := range types.FeedBalanceAccountNames {
			metrics.On("Cleanup",
				accountName,
				balances.Addresses[accountName].String(),
				feedConfig.GetID(),
				chainConfig.GetChainID(),
				feedConfig.GetContractStatus(),
				feedConfig.GetContractType(),
				feedConfig.GetName(),
				feedConfig.GetPath(),
				chainConfig.GetNetworkID(),
				chainConfig.GetNetworkName(),
			)
		}
		exporter.Cleanup(ctx)
	})
}
