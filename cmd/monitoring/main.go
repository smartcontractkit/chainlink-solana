package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-relay/pkg/monitoring/config"

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

	if chainConfig.RunMode == monitoring.MonitorMode {
		runMonitor(chainConfig, log)
	} else if chainConfig.RunMode == monitoring.IngestorMode {
		runIngestor(chainConfig, log)
	}
}

func runMonitor(chainConfig monitoring.SolanaConfig, log logger.Logger) {
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

func runIngestor(chainConfig monitoring.SolanaConfig, log logger.Logger) {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := ws.Connect(rootCtx, chainConfig.WSEndpoint)
	if err != nil {
		log.Fatalw("failed to connect to WS server", "error", err)
	}
	defer client.Close()

	cfg, err := config.Parse()
	if err != nil {
		log.Fatalw("failed to parse generic configuration", "error", err)
	}

	chainMetrics := relayMonitoring.NewChainMetrics(chainConfig)
	ingestorMetrics := monitoring.NewIngestorMetrics(chainConfig)

	producer, err := relayMonitoring.NewProducer(rootCtx, logger.With(log, "component", "producer"), cfg.Kafka)
	if err != nil {
		log.Fatalw("failed to create kafka producer", "error", err)
	}
	producer = relayMonitoring.NewInstrumentedProducer(producer, chainMetrics)

	schemaRegistry := relayMonitoring.NewSchemaRegistry(cfg.SchemaRegistry, log)

	stateSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.StatesKafkaTopic), monitoring.StateAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare state schema", "error", err)
	}
	transmissionSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.TransmissionsKafkaTopic), monitoring.TransmissionAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare transmissions schema", "error", err)
	}
	eventsSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.EventsKafkaTopic), monitoring.EventsAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare events schema", "error", err)
	}
	blockSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.BlocksKafkaTopic), monitoring.BlockAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare blocks schema", "error", err)
	}

	rddSource := relayMonitoring.NewRDDSource(
		cfg.Feeds.URL, monitoring.SolanaFeedsParser, cfg.Feeds.IgnoreIDs,
		cfg.Nodes.URL, monitoring.SolanaNodesParser,
		logger.With(log, "component", "rdd-source"),
	)

	rddPoller := relayMonitoring.NewSourcePoller(
		rddSource,
		logger.With(log, "component", "rdd-poller"),
		cfg.Feeds.RDDPollInterval,
		cfg.Feeds.RDDReadTimeout,
		0, // no buffering!
	)

	manager := relayMonitoring.NewManager(
		logger.With(log, "component", "manager"),
		rddPoller,
	)

	// Configure HTTP server
	httpServer := relayMonitoring.NewHTTPServer(
		rootCtx,
		cfg.HTTP.Address,
		logger.With(log, "component", "http-server"),
	)
	httpServer.Handle("/debug", manager.HTTPHandler())
	httpServer.Handle("/metrics", ingestorMetrics.HTTPHandler())

	processor := monitoring.NewIngestor(
		chainConfig,
		client,
		rpc.CommitmentConfirmed,
		producer,
		stateSchema,
		transmissionSchema,
		eventsSchema,
		blockSchema,
		ingestorMetrics,
		log,
	)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer log.Infow("rdd poller stopped")
		defer wg.Done()
		rddPoller.Run(rootCtx)
	}()

	wg.Add(1)
	go func() {
		defer log.Infow("http server stopped")
		defer wg.Done()
		httpServer.Run(rootCtx)
	}()

	// Handle signals from the OS
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer log.Infow("signals listener stopped")
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
		defer log.Infow("manager stopped")
		manager.Run(rootCtx, processor.Run)
	}()

	wg.Wait()
}
