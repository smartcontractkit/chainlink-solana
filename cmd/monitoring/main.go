package main

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
)

func main() {
	coreLog, closeLggr := logger.NewLogger()
	defer func() {
		if closeLggr != nil {
			_ = closeLggr()
		}
	}()
	log := logWrapper{coreLog}

	chainConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana-specific config", "error", err)
	}

	client := rpc.New(chainConfig.RPCEndpoint)

	envelopeSourceFactory := monitoring.NewEnvelopeSourceFactory(
		client,
		logWrapper{coreLog.With("component", "source-envelope")},
	)
	txResultsSourceFactory := monitoring.NewTxResultsSourceFactory(
		client,
		logWrapper{coreLog.With("component", "source-txresults")},
	)

	entrypoint, err := relayMonitoring.NewEntrypoint(
		context.Background(),
		log,
		chainConfig,
		envelopeSourceFactory,
		txResultsSourceFactory,
		monitoring.SolanaFeedParser,
	)
	if err != nil {
		log.Fatalw("failed to build entrypoint", "error", err)
		return
	}

	balancesSourceFactory := monitoring.NewBalancesSourceFactory(
		client,
		log.With("component", "source-balances"),
	)
	if entrypoint.Config.Feature.TestOnlyFakeReaders {
		balancesSourceFactory = monitoring.NewFakeBalancesSourceFactory(log.With("component", "fake-balances-source"))
	}
	entrypoint.SourceFactories = append(entrypoint.SourceFactories, balancesSourceFactory)

	promExporterFactory := monitoring.NewPrometheusExporterFactory(
		log.With("component", "solana-prom-exporter"),
		monitoring.NewMetrics(log.With("component", "solana-metrics")),
	)
	entrypoint.ExporterFactories = append(entrypoint.ExporterFactories, promExporterFactory)

	entrypoint.Run()
	log.Infow("monitor stopped")
}

// adapt core logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
