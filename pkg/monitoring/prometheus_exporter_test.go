package monitoring

import (
	"context"
	"testing"
	"time"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/require"
)

func TestPrometheusExporter(t *testing.T) {
	t.Run("it should export balance updates then clean up", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		metrics := new(mocks.Metrics)
		metrics.Test(t)
		factory := NewPrometheusExporterFactory(logWrapper{logger.NullLogger}, metrics)

		chainConfig := generateChainConfig()
		feedConfig := generateFeedConfig()
		exporter, err := factory.NewExporter(chainConfig, feedConfig)
		require.NoError(t, err)

		balances := generateBalances()

		for _, accountName := range BalanceAccountNames {
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

		for _, accountName := range BalanceAccountNames {
			metrics.On("Cleanup",
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

// Helpers

// adapt core/logger.Logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
