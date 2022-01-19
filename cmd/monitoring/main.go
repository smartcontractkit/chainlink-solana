package main

import (
	"context"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"
	"go.uber.org/zap/zapcore"
)

func main() {
	ctx := context.Background()

	log := logger.NewLogger(loggerConfig{})

	solanaConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana specific configuration", "error", err)
	}

	solanaSourceFactory := monitoring.NewSolanaSourceFactory(log.With("component", "source"))

	monitoring.Facade(
		ctx,
		log,
		solanaConfig,
		solanaSourceFactory,
		monitoring.SolanaFeedParser,
	)

	log.Info("monitor stopped")
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
