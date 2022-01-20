package main

import (
	"context"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"
	"go.uber.org/zap/zapcore"
)

func main() {
	ctx := context.Background()

	coreLog := logger.NewLogger(loggerConfig{})
	log := logWrapper{coreLog}

	solanaConfig, err := monitoring.ParseSolanaConfig()
	if err != nil {
		log.Fatalw("failed to parse solana specific configuration", "error", err)
	}

	solanaSourceFactory := monitoring.NewSolanaSourceFactory(logWrapper{coreLog.With("component", "source")})

	relayMonitoring.Facade(
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

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) Criticalw(format string, values ...interface{}) {
	l.Logger.CriticalW(format, values...)
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}
