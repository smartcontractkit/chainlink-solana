package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
)

func main() {
	bgCtx, cancelBgCtx := context.WithCancel(context.Background())
	defer cancelBgCtx()
	var wg sync.WaitGroup

	log := logger.NewLogger(loggerConfig{})

	cfg, err := monitoring.ParseConfig(bgCtx)
	if err != nil {
		log.Fatalf("failed to parse configuration: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:        cfg.Http.Address,
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return bgCtx },
	}
	defer server.Close()
	wg.Add(1)
	go func() {
		if err := http.ListenAndServe(cfg.Http.Address, nil); err != nil {
			log.Fatalf("failed to start http server with address %s: error %v", cfg.Http.Address, err)
		}
	}()

	client := rpc.New(cfg.Solana.RPCEndpoint)

	schemaRegistry := monitoring.NewSchemaRegistry(cfg.SchemaRegistry)
	trSchema, err := schemaRegistry.EnsureSchema("transmission-value", monitoring.TransmissionAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare transmission schema with error: %v", err)
	}
	stSchema, err := schemaRegistry.EnsureSchema("config_set-value", monitoring.ConfigSetAvroSchema)
	if err != nil {
		log.Fatalf("failed to prepare config_set schema with error: %v", err)
	}

	producer, err := monitoring.NewProducer(ctx, cfg.Kafka)
	if err != nil {
		log.Fatalf("failed to create kafka producer with error: %v", err)
	}

	trReader := monitoring.NewTransmissionReader(client)
	stReader := monitoring.NewStateReader(client)

	monitor := monitoring.NewMultiFeedMonitor(
		cfg.Solana,
		trReader, stReader,
		trSchema, stSchema,
		producer,
		cfg.Feeds,
		monitoring.DefaultMetrics,
	)
	go monitor.Start(ctx)

	signalsCh := make(chan os.Signal, 1)
	signal.Notify(signalsCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signalsCh
	log.Printf("Received signal %v. Stopping\n", sig)
}
