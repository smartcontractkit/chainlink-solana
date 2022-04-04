package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-relay/pkg/monitoring/config"
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

	chainConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		coreLog.Fatalw("failed to parse solana-specific config", "error", err)
	}

	if chainConfig.RunMode == "monitor" {
		runMonitor(chainConfig, coreLog)
	} else if chainConfig.RunMode == "ingestor" {
		runIngestor(chainConfig, coreLog)
	}
}

func runMonitor(chainConfig monitoring.SolanaConfig, coreLog logger.Logger) {
	l := logWrapper{coreLog}

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

func runIngestor(chainConfig monitoring.SolanaConfig, coreLog logger.Logger) {
	wg := &sync.WaitGroup{}
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logWrapper{coreLog}

	client, err := ws.Connect(rootCtx, chainConfig.WSEndpoint)
	if err != nil {
		log.Fatalw("failed to connect to WS server", "error", err)
	}
	defer client.Close()

	cfg, err := config.Parse()
	if err != nil {
		log.Fatalw("failed to parse generic configuration", "error", err)
	}

	producer, err := relayMonitoring.NewProducer(rootCtx, log.With("component", "producer"), cfg.Kafka)
	if err != nil {
		log.Fatalw("failed to create kafka producer", "error", err)
	}
	// TODO instrumented producer

	schemaRegistry := relayMonitoring.NewSchemaRegistry(cfg.SchemaRegistry, log)

	stateSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.StateKafkaTopic), monitoring.StateAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare state schema", "error", err)
	}
	transmissionsSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.TransmissionsKafkaTopic), monitoring.TransmissionsAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare transmissions schema", "error", err)
	}
	eventsSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.EventsKafkaTopic), monitoring.EventsAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare transmissions schema", "error", err)
	}

	rddSource := relayMonitoring.NewRDDSource(
		cfg.Feeds.URL,
		monitoring.SolanaFeedParser,
		log.With("component", "rdd-source"),
		cfg.Feeds.IgnoreIDs,
	)

	rddPoller := relayMonitoring.NewSourcePoller(
		rddSource,
		log.With("component", "rdd-poller"),
		cfg.Feeds.RDDPollInterval,
		cfg.Feeds.RDDReadTimeout,
		0, // no buffering!
	)

	manager := relayMonitoring.NewManager(
		log.With("component", "manager"),
		rddPoller,
	)

	// Configure HTTP server
	httpServer := relayMonitoring.NewHTTPServer(rootCtx, cfg.HTTP.Address, log.With("component", "http-server"))
	httpServer.Handle("/debug", manager.HTTPHandler())
	// TODO (dru) add metrics

	processor := monitoring.NewIngestor(
		chainConfig,
		client,
		rpc.CommitmentConfirmed,
		producer,
		stateSchema,
		transmissionsSchema,
		eventsSchema,
		log,
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		rddPoller.Run(rootCtx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		httpServer.Run(rootCtx)
	}()

	// Handle signals from the OS
	wg.Add(1)
	go func() {
		defer wg.Done()
		osSignalsCh := make(chan os.Signal, 1)
		signal.Notify(osSignalsCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case sig := <-osSignalsCh:
			log.Infow("received signal. Stopping", "signal", sig)
			cancel()
		case <-rootCtx.Done():
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.Run(rootCtx, func(localCtx context.Context, feeds []relayMonitoring.FeedConfig) {
			processor.Run(localCtx, feeds)
		})
	}()

	wg.Wait()
}

// adapt core logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
