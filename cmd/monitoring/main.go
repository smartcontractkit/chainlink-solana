package main

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

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

	monitor, err := commonMonitoring.NewMonitor(
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

	// per-feed sources
	feedBalancesSourceFactory := monitoring.NewFeedBalancesSourceFactory(
		chainReader,
		logger.With(log, "component", "source-feed-balances"),
	)
	txDetailsSourceFactory := monitoring.NewTxDetailsSourceFactory(
		chainReader,
		logger.With(log, "component", "source-tx-details"),
	)
	monitor.SourceFactories = append(monitor.SourceFactories,
		feedBalancesSourceFactory,
		txDetailsSourceFactory,
	)

	// network sources
	nodeBalancesSourceFactory := monitoring.NewNodeBalancesSourceFactory(
		chainReader,
		logger.With(log, "component", "source-node-balances"),
	)
	monitor.NetworkSourceFactories = append(monitor.NetworkSourceFactories,
		nodeBalancesSourceFactory,
	)

	// per-feed exporters
	feedBalancesExporterFactory := exporter.NewFeedBalancesFactory(
		logger.With(log, "component", "solana-prom-exporter"),
		metrics.NewFeedBalances(logger.With(log, "component", "solana-metrics")),
	)
	reportObservationsFactory := exporter.NewReportObservationsFactory(
		logger.With(log, "component", "solana-prome-exporter"),
		metrics.NewReportObservations(logger.With(log, "component", "solana-metrics")),
	)
	monitor.ExporterFactories = append(monitor.ExporterFactories,
		feedBalancesExporterFactory,
		reportObservationsFactory,
	)

	// network exporters
	nodeBalancesExporterFactory := exporter.NewNodeBalancesFactory(
		logger.With(log, "component", "solana-prom-exporter"),
		metrics.NewNodeBalances,
	)
	monitor.NetworkExporterFactories = append(monitor.NetworkExporterFactories,
		nodeBalancesExporterFactory,
	)

	monitor.Run()
	log.Infow("monitor stopped")
}
