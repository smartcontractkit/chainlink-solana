package main

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
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

	chainConfig, err := monitoring.ParseSolanaConfig()
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
		monitoring.SolanaFeedsParser,
		monitoring.SolanaNodesParser,
	)
	if err != nil {
		log.Fatalw("failed to build monitor", "error", err)
		return
	}

	balancesSourceFactory := monitoring.NewBalancesSourceFactory(
		chainReader,
		logger.With(log, "component", "source-balances"),
	)
	monitor.SourceFactories = append(monitor.SourceFactories, balancesSourceFactory)

	promExporterFactory := monitoring.NewPrometheusExporterFactory(
		logger.With(log, "component", "solana-prom-exporter"),
		monitoring.NewMetrics(logger.With(log, "component", "solana-metrics")),
	)
	monitor.ExporterFactories = append(monitor.ExporterFactories, promExporterFactory)

	monitor.Run()
	log.Infow("monitor stopped")
}
