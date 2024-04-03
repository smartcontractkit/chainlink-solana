package main

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	relayMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/exporter"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
)

func main() {
	log, err := logger.New()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if serr := log.Sync(); serr != nil {
			fmt.Printf("Error while closing Logger: %v\n", serr)
		}
	}()

	chainConfig, err := config.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana-specific config", "error", err)
	}

	client := rpc.New(chainConfig.RPCEndpoint)
	chainReader := monitoring.NewChainReader(client)

	envelopeSourceFactory := monitoring.NewEnvelopeSourceFactory(
		chainReader,
		logger.With(log, "component", "source-envelope"),
	)
	txResultsSourceFactory := monitoring.NewTxResultsSourceFactory(
		chainReader,
		logger.With(log, "component", "source-txresults"),
	)

	monitor, err := relayMonitoring.NewMonitor(
		context.Background(),
		log,
		chainConfig,
		envelopeSourceFactory,
		txResultsSourceFactory,
		config.SolanaFeedsParser,
		config.SolanaNodesParser,
	)
	if err != nil {
		log.Fatalw("failed to build monitor", "error", err)
		return
	}

	balancesSourceFactory := monitoring.NewFeedBalancesSourceFactory(
		chainReader,
		logger.With(log, "component", "source-balances"),
	)
	monitor.SourceFactories = append(monitor.SourceFactories, balancesSourceFactory)

	promExporterFactory := exporter.NewFeedBalancePrometheusExporterFactory(
		logger.With(log, "component", "solana-prom-exporter"),
		metrics.NewMetrics(logger.With(log, "component", "solana-metrics")),
	)
	monitor.ExporterFactories = append(monitor.ExporterFactories, promExporterFactory)

	monitor.Run()
	log.Infow("monitor stopped")
}
