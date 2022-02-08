package main

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"
	"go.uber.org/zap/zapcore"
)

func main() {
	coreLog := logger.NewLogger(loggerConfig{}).With("project", "solana")
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

	balancesExporterFactory := monitoring.NewPrometheusExporterFactory(
		log.With("component", "balances-prometheus-exporter"),
		monitoring.DefaultMetrics,
	)
	entrypoint.ExporterFactories = append(entrypoint.ExporterFactories, balancesExporterFactory)

	entrypoint.Run()
	log.Infow("monitor stopped")
}

// logger config

type loggerConfig struct{}

var _ logger.Config = loggerConfig{}

func (l loggerConfig) RootDir() string {
	return "" // Not logging to disk.
}

func (l loggerConfig) JSONConsole() bool {
	return false // Logs lines are JSON formatted
}

func (l loggerConfig) LogToDisk() bool {
	return false
}

func (l loggerConfig) LogLevel() zapcore.Level {
	return zapcore.InfoLevel // And just like that, we now depend on zapcore!
}

func (l loggerConfig) LogUnixTimestamps() bool {
	return false // log timestamp in ISO8601
}

// adapt core logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) Criticalw(format string, values ...interface{}) {
	l.Logger.CriticalW(format, values...)
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
