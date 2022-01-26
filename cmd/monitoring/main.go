package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	relayConfig "github.com/smartcontractkit/chainlink-relay/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"
	"go.uber.org/zap/zapcore"
)

func main() {
	coreLog := logger.NewLogger(loggerConfig{}).With("project", "solana")
	log := logWrapper{coreLog}

	chainConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana-specific config", "error", err)
	}

	sourceFactory := monitoring.NewSolanaSourceFactory(
		chainConfig,
		logWrapper{coreLog.With("component", "source")},
	)

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	bgCtx, cancelBgCtx := context.WithCancel(context.Background())
	defer cancelBgCtx()

	cfg, err := relayConfig.Parse()
	if err != nil {
		log.Fatalw("failed to parse generic configuration", "error", err)
	}

	schemaRegistry := relayMonitoring.NewSchemaRegistry(cfg.SchemaRegistry, log)

	transmissionSchema, err := schemaRegistry.EnsureSchema(cfg.Kafka.TransmissionTopic+"-value", relayMonitoring.TransmissionAvroSchema)
	if err != nil {
		log.Fatalw("failed to prepare transmission schema", "error", err)
	}
	configSetSimplifiedSchema, err := schemaRegistry.EnsureSchema(cfg.Kafka.ConfigSetSimplifiedTopic+"-value", relayMonitoring.ConfigSetSimplifiedAvroSchema)
	if err != nil {
		log.Fatalw("failed to prepare config_set_simplified schema", "error", err)
	}

	producer, err := relayMonitoring.NewProducer(bgCtx, log.With("component", "producer"), cfg.Kafka)
	if err != nil {
		log.Fatalw("failed to create kafka producer", "error", err)
	}

	if cfg.Feature.TestOnlyFakeReaders {
		sourceFactory = relayMonitoring.NewRandomDataSourceFactory(bgCtx, wg, log.With("component", "rand-source"))
	}

	balancesSourceFactory := monitoring.NewBalancesSourceFactory(chainConfig, log.With("component", "balances-source"))
	if cfg.Feature.TestOnlyFakeReaders {
		balancesSourceFactory = monitoring.NewFakeBalancesSourceFactory(log.With("component", "fake-balances-source"))
	}

	metrics := relayMonitoring.DefaultMetrics

	prometheusExporterFactory := relayMonitoring.NewPrometheusExporterFactory(
		log.With("component", "prometheus-exporter"),
		metrics,
	)
	kafkaExporterFactory := relayMonitoring.NewKafkaExporterFactory(
		log.With("component", "kafka-exporter"),
		producer,

		transmissionSchema,
		configSetSimplifiedSchema,

		cfg.Kafka.ConfigSetSimplifiedTopic,
		cfg.Kafka.TransmissionTopic,
	)

	balancesPrometheusExporterFactory := monitoring.NewPrometheusExporterFactory(
		log.With("component", "balances-prometheus-exporter"),
		monitoring.DefaultMetrics,
	)

	monitor := relayMonitoring.NewMultiFeedMonitor(
		chainConfig,
		log,
		[]relayMonitoring.SourceFactory{sourceFactory, balancesSourceFactory},
		[]relayMonitoring.ExporterFactory{prometheusExporterFactory, kafkaExporterFactory, balancesPrometheusExporterFactory},
	)

	rddSource := relayMonitoring.NewRDDSource(cfg.Feeds.URL, monitoring.SolanaFeedParser)
	if cfg.Feature.TestOnlyFakeRdd {
		// Generate between 2 and 10 random feeds every RDDPollInterval.
		rddSource = monitoring.NewFakeRDDSource(2, 10)
	}
	rddPoller := relayMonitoring.NewSourcePoller(
		rddSource,
		log.With("component", "rdd-poller"),
		cfg.Feeds.RDDPollInterval,
		cfg.Feeds.RDDReadTimeout,
		0, // no buffering!
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		rddPoller.Run(bgCtx)
	}()

	manager := relayMonitoring.NewManager(
		log.With("component", "manager"),
		rddPoller,
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.Run(bgCtx, monitor.Run)
	}()

	// Configure HTTP server
	http := relayMonitoring.NewHttpServer(bgCtx, cfg.Http.Address, log.With("component", "http-server"))
	http.Handle("/metrics", metrics.HTTPHandler())
	http.Handle("/debug", manager.HTTPHandler())
	wg.Add(1)
	go func() {
		defer wg.Done()
		http.Run(bgCtx)
	}()

	// Handle signals from the OS
	osSignalsCh := make(chan os.Signal, 1)
	signal.Notify(osSignalsCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-osSignalsCh
	log.Infow("received signal. Stopping", "signal", sig)

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

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) Criticalw(format string, values ...interface{}) {
	l.Logger.CriticalW(format, values...)
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
