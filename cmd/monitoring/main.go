package main

import (
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
		make(chan struct{}),
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
	slotHeightSourceFactory := monitoring.NewSlotHeightSourceFactory(
		chainReader,
		logger.With(log, "component", "source-slot-height"),
	)
	networkFeesSourceFactory := monitoring.NewNetworkFeesSourceFactory(
		chainReader,
		logger.With(log, "component", "source-network-fees"),
	)
	monitor.NetworkSourceFactories = append(monitor.NetworkSourceFactories,
		nodeBalancesSourceFactory,
		slotHeightSourceFactory,
		networkFeesSourceFactory,
	)

	// exporter names
	promExporter := "solana-prom-exporter"
	promMetrics := "solana-metrics"

	// per-feed exporters
	feedBalancesExporterFactory := exporter.NewFeedBalancesFactory(
		logger.With(log, "component", promExporter),
		metrics.NewFeedBalances(logger.With(log, "component", promMetrics)),
	)
	reportObservationsFactory := exporter.NewReportObservationsFactory(
		logger.With(log, "component", promExporter),
		metrics.NewReportObservations(logger.With(log, "component", promMetrics)),
	)
	feesFactory := exporter.NewFeesFactory(
		logger.With(log, "component", promExporter),
		metrics.NewFees(logger.With(log, "component", promMetrics)),
	)
	nodeSuccessFactory := exporter.NewNodeSuccessFactory(
		logger.With(log, "component", promExporter),
		metrics.NewNodeSuccess(logger.With(log, "component", promMetrics)),
	)
	monitor.ExporterFactories = append(monitor.ExporterFactories,
		feedBalancesExporterFactory,
		reportObservationsFactory,
		feesFactory,
		nodeSuccessFactory,
	)

	// network exporters
	nodeBalancesExporterFactory := exporter.NewNodeBalancesFactory(
		logger.With(log, "component", promExporter),
		metrics.NewNodeBalances,
	)
	slotHeightExporterFactory := exporter.NewSlotHeightFactory(
		logger.With(log, "component", promExporter),
		metrics.NewSlotHeight(logger.With(log, "component", promMetrics)),
	)
	networkFeesExporterFactory := exporter.NewNetworkFeesFactory(
		logger.With(log, "component", promExporter),
		metrics.NewNetworkFees(logger.With(log, "component", promMetrics)),
	)
	monitor.NetworkExporterFactories = append(monitor.NetworkExporterFactories,
		nodeBalancesExporterFactory,
		slotHeightExporterFactory,
		networkFeesExporterFactory,
	)

	monitor.Run()
	log.Infow("monitor stopped")
}
