package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
)

func main() {
	coreLog, closeLggr := logger.NewLogger()
	defer func() {
		if err := closeLggr(); err != nil {
			log.Println(fmt.Sprintf("Error while closing Logger: %v", err))
		}
	}()
	l := logWrapper{coreLog}

	chainConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		l.Fatalw("failed to parse solana-specific config", "error", err)
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

	monitor, err := relayMonitoring.NewMonitor(
		context.Background(),
		l,
		chainConfig,
		envelopeSourceFactory,
		txResultsSourceFactory,
		monitoring.SolanaFeedParser,
	)
	if err != nil {
		l.Fatalw("failed to build monitor", "error", err)
		return
	}

	balancesSourceFactory := monitoring.NewBalancesSourceFactory(
		client,
		l.With("component", "source-balances"),
	)
	monitor.SourceFactories = append(monitor.SourceFactories, balancesSourceFactory)

	promExporterFactory := monitoring.NewPrometheusExporterFactory(
		l.With("component", "solana-prom-exporter"),
		monitoring.NewMetrics(l.With("component", "solana-metrics")),
	)
	monitor.ExporterFactories = append(monitor.ExporterFactories, promExporterFactory)

	monitor.Run()
	l.Infow("monitor stopped")
}

// adapt core logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
