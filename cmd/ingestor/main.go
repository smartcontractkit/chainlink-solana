package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-relay/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/ingestor"
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

	chainConfig, err := ingestor.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana-specific config", "error", err)
	}

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
	ingestorMetrics := ingestor.NewIngestorMetrics(chainConfig)

	producer, err := relayMonitoring.NewProducer(rootCtx, logger.With(log, "component", "producer"), cfg.Kafka)
	if err != nil {
		log.Fatalw("failed to create kafka producer", "error", err)
	}
	producer = relayMonitoring.NewInstrumentedProducer(producer, chainMetrics)

	schemaRegistry := relayMonitoring.NewSchemaRegistry(cfg.SchemaRegistry, log)

	stateSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.StatesKafkaTopic), ingestor.StateAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare state schema", "error", err)
	}
	transmissionSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.TransmissionsKafkaTopic), ingestor.TransmissionAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare transmissions schema", "error", err)
	}
	eventsSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.EventsKafkaTopic), ingestor.EventsAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare events schema", "error", err)
	}
	blockSchema, err := schemaRegistry.EnsureSchema(
		relayMonitoring.SubjectFromTopic(chainConfig.BlocksKafkaTopic), ingestor.BlockAvroSchema)
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

	processor := ingestor.New(
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

	var subs utils.Subprocesses
	subs.Go(func() {
		defer log.Infow("rdd poller stopped")
		rddPoller.Run(rootCtx)
	})
	subs.Go(func() {
		defer log.Infow("http server stopped")
		httpServer.Run(rootCtx)
	})
	// Handle signals from the OS
	subs.Go(func() {
		defer log.Infow("signals listener stopped")
		osSignalsCh := make(chan os.Signal, 1)
		signal.Notify(osSignalsCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case sig := <-osSignalsCh:
			log.Infow("received signal. Stopping", "signal", sig)
			cancel()
		case <-rootCtx.Done():
		}
	})
	subs.Go(func() {
		defer log.Infow("manager stopped")
		manager.Run(rootCtx, processor.Run)
	})
	subs.Wait()
}
