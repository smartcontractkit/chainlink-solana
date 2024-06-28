package exporter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestFeedBalances(t *testing.T) {
	t.Parallel()

	t.Run("it should export balance updates then clean up", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		mockMetrics := mocks.NewFeedBalances(t)
		factory := NewFeedBalancesFactory(testutils.NewNullLogger(), mockMetrics)

		chainConfig := testutils.GenerateChainConfig()
		feedConfig := testutils.GenerateFeedConfig()
		exporter, err := factory.NewExporter(commonMonitoring.ExporterParams{ChainConfig: chainConfig, FeedConfig: feedConfig, Nodes: []commonMonitoring.NodeConfig{}})
		require.NoError(t, err)

		balances := testutils.GenerateBalances()

		// return all gauges exist
		mockMetrics.On("Exists", mock.Anything).Return(nil, true)

		for _, accountName := range types.FeedBalanceAccountNames {
			mockMetrics.On("SetBalance",
				balances.Values[accountName],
				accountName,
				metrics.FeedInput{
					AccountAddress: balances.Addresses[accountName].String(),
					FeedID:         feedConfig.GetID(),
					ChainID:        chainConfig.GetChainID(),
					ContractStatus: feedConfig.GetContractStatus(),
					ContractType:   feedConfig.GetContractType(),
					FeedName:       feedConfig.GetName(),
					FeedPath:       feedConfig.GetPath(),
					NetworkID:      chainConfig.GetNetworkID(),
					NetworkName:    chainConfig.GetNetworkName(),
				},
			)
		}
		exporter.Export(ctx, balances)

		for _, accountName := range types.FeedBalanceAccountNames {
			mockMetrics.On("Cleanup", accountName, metrics.FeedInput{
				AccountAddress: balances.Addresses[accountName].String(),
				FeedID:         feedConfig.GetID(),
				ChainID:        chainConfig.GetChainID(),
				ContractStatus: feedConfig.GetContractStatus(),
				ContractType:   feedConfig.GetContractType(),
				FeedName:       feedConfig.GetName(),
				FeedPath:       feedConfig.GetPath(),
				NetworkID:      chainConfig.GetNetworkID(),
				NetworkName:    chainConfig.GetNetworkName(),
			})
		}
		exporter.Cleanup(ctx)
	})
}
