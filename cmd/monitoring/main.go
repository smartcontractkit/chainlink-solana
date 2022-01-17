package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink/core/logger"
	"go.uber.org/zap/zapcore"
)

func main() {
	bgCtx, cancelBgCtx := context.WithCancel(context.Background())
	defer cancelBgCtx()
	wg := &sync.WaitGroup{}

	log := logger.NewLogger(loggerConfig{})

	cfg, err := config.Parse()
	if err != nil {
		log.Fatalw("failed to parse configuration", "error", err)
	}

	client := rpc.New(cfg.Solana.RPCEndpoint)

	schemaRegistry := monitoring.NewSchemaRegistry(cfg.SchemaRegistry, log)
	transmissionSchema, err := schemaRegistry.EnsureSchema(cfg.Kafka.TransmissionTopic+"-value", monitoring.TransmissionAvroSchema)
	if err != nil {
		log.Fatalw("failed to prepare transmission schema", "error", err)
	}
	configSetSchema, err := schemaRegistry.EnsureSchema(cfg.Kafka.ConfigSetTopic+"-value", monitoring.ConfigSetAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare config_set schema", "error", err)
	}
	configSetSimplifiedSchema, err := schemaRegistry.EnsureSchema(cfg.Kafka.ConfigSetSimplifiedTopic+"-value", monitoring.ConfigSetSimplifiedAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare config_set_simplified schema", "error", err)
	}

	producer, err := monitoring.NewProducer(bgCtx, log.With("component", "producer"), cfg.Kafka)
	if err != nil {
		log.Fatalf("failed to create kafka producer", "error", err)
	}

	var transmissionReader, stateReader monitoring.AccountReader
	if cfg.Feature.TestOnlyFakeReaders {
		transmissionReader = monitoring.NewRandomDataReader(bgCtx, wg, "transmission", log.With("component", "rand-reader", "account", "transmissions"))
		stateReader = monitoring.NewRandomDataReader(bgCtx, wg, "state", log.With("component", "rand-reader", "account", "state"))
	} else {
		transmissionReader = monitoring.NewTransmissionReader(client)
		stateReader = monitoring.NewStateReader(client)
	}

	monitor := monitoring.NewMultiFeedMonitor(
		cfg.Solana,

		log,
		transmissionReader, stateReader,
		producer,
		monitoring.DefaultMetrics,

		cfg.Kafka.ConfigSetTopic,
		cfg.Kafka.ConfigSetSimplifiedTopic,
		cfg.Kafka.TransmissionTopic,

		configSetSchema,
		configSetSimplifiedSchema,
		transmissionSchema,
	)

	source := monitoring.NewRDDSource(cfg.Feeds.URL)
	if cfg.Feature.TestOnlyFakeRdd {
		// Generate between 2 and 10 random feeds every RDDPollInterval.
		source = monitoring.NewFakeRDDSource(2, 10)
	}
	rddPoller := monitoring.NewSourcePoller(
		source,
		log.With("component", "rdd-poller"),
		cfg.Feeds.RDDPollInterval,
		cfg.Feeds.RDDReadTimeout,
		0, // no buffering!
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		rddPoller.Start(bgCtx)
	}()

	manager := monitoring.NewManager(
		log.With("component", "manager"),
		rddPoller,
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.Start(bgCtx, wg, monitor.Start)
	}()

	// Configure HTTP server
	http := monitoring.NewHttpServer(bgCtx, cfg.Http.Address, log.With("component", "http-server"))
	http.Handle("/metrics", metrics.HTTPHandler())
	http.Handle("/debug", manager.HTTPHandler())
	wg.Add(1)
	go func() {
		defer wg.Done()
		http.Start(bgCtx, wg)
	}()

	// Handle signals from the OS
	osSignalsCh := make(chan os.Signal, 1)
	signal.Notify(osSignalsCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-osSignalsCh
	log.Infof("received signal '%v'. Stopping", sig)

	cancelBgCtx()
	wg.Wait()
	log.Info("process stopped")
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
