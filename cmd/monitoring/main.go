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
	coreLog, closeLog := logger.NewLogger()
	defer func() {
		if err := closeLog(); err != nil {
			log.Println(fmt.Sprintf("Error while closing Logger: %v", err))
		}
	}()
	log := logWrapper{coreLog}

	chainConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana-specific config", "error", err)
	}

	client := rpc.New(chainConfig.RPCEndpoint)
	chainReader := monitoring.NewChainReader(client)

	envelopeSourceFactory := monitoring.NewEnvelopeSourceFactory(
		chainReader,
		logWrapper{coreLog.With("component", "source-envelope")},
	)
	txResultsSourceFactory := monitoring.NewTxResultsSourceFactory(
		chainReader,
		logWrapper{coreLog.With("component", "source-txresults")},
	)

	monitor, err := relayMonitoring.NewMonitor(
		context.Background(),
		log,
		chainConfig,
		envelopeSourceFactory,
		txResultsSourceFactory,
		monitoring.SolanaFeedsParser,
		monitoring.SolanaNodesParser,
	)
	if err != nil {
		log.Fatalw("failed to build monitor", "error", err)
		return
	}

	balancesSourceFactory := monitoring.NewBalancesSourceFactory(
		chainReader,
		log.With("component", "source-balances"),
	)
	monitor.SourceFactories = append(monitor.SourceFactories, balancesSourceFactory)

	promExporterFactory := monitoring.NewPrometheusExporterFactory(
		log.With("component", "solana-prom-exporter"),
		monitoring.NewMetrics(log.With("component", "solana-metrics")),
	)
	monitor.ExporterFactories = append(monitor.ExporterFactories, promExporterFactory)

	monitor.Run()
	log.Infow("monitor stopped")
}

// adapt core logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
