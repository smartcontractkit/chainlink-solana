package monitoring

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

type SolanaConfig struct {
	RunMode string // either "monitor" or "ingestor"

	RPCEndpoint  string
	WSEndpoint   string
	NetworkName  string
	NetworkID    string
	ChainID      string
	ReadTimeout  time.Duration
	PollInterval time.Duration

	// Solana-specific kafka topics
	StateKafkaTopic         string
	TransmissionsKafkaTopic string
	EventsKafkaTopic        string
}

var _ relayMonitoring.ChainConfig = SolanaConfig{}

func (s SolanaConfig) GetRPCEndpoint() string         { return s.RPCEndpoint }
func (s SolanaConfig) GetNetworkName() string         { return s.NetworkName }
func (s SolanaConfig) GetNetworkID() string           { return s.NetworkID }
func (s SolanaConfig) GetChainID() string             { return s.ChainID }
func (s SolanaConfig) GetReadTimeout() time.Duration  { return s.ReadTimeout }
func (s SolanaConfig) GetPollInterval() time.Duration { return s.PollInterval }

func (s SolanaConfig) ToMapping() map[string]interface{} {
	return map[string]interface{}{
		"network_name": s.NetworkName,
		"network_id":   s.NetworkID,
		"chain_id":     s.ChainID,
	}
}

func ParseSolanaConfig() (SolanaConfig, error) {
	cfg := SolanaConfig{}

	if err := parseFlags(&cfg); err != nil {
		return cfg, err
	}

	if err := parseEnvVars(&cfg); err != nil {
		return cfg, err
	}

	applyDefaults(&cfg)

	err := validateConfig(cfg)
	return cfg, err
}

func parseFlags(cfg *SolanaConfig) error {
	runMode := flag.String("run-mode", "monitor", "either 'monitor' or 'ingestor'")
	flag.Parse()
	if runMode == nil {
		return fmt.Errorf("must set --run-mode")
	}
	if *runMode != "monitor" && *runMode != "ingestor" {
		return fmt.Errorf("--run-mode needs to be either 'monitor' or 'ingestor', '%s' not supported", cfg.RunMode)
	}
	cfg.RunMode = *runMode
	return nil
}

func parseEnvVars(cfg *SolanaConfig) error {
	if value, isPresent := os.LookupEnv("SOLANA_RPC_ENDPOINT"); isPresent {
		cfg.RPCEndpoint = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_WS_ENDPOINT"); isPresent {
		cfg.WSEndpoint = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_NETWORK_NAME"); isPresent {
		cfg.NetworkName = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_NETWORK_ID"); isPresent {
		cfg.NetworkID = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_CHAIN_ID"); isPresent {
		cfg.ChainID = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_READ_TIMEOUT"); isPresent {
		readTimeout, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("failed to parse env var SOLANA_READ_TIMEOUT, see https://pkg.go.dev/time#ParseDuration: %w", err)
		}
		cfg.ReadTimeout = readTimeout
	}
	if value, isPresent := os.LookupEnv("SOLANA_POLL_INTERVAL"); isPresent {
		pollInterval, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("failed to parse env var SOLANA_POLL_INTERVAL, see https://pkg.go.dev/time#ParseDuration: %w", err)
		}
		cfg.PollInterval = pollInterval
	}
	if value, isPresent := os.LookupEnv("SOLANA_STATE_KAFKA_TOPIC"); isPresent {
		cfg.StateKafkaTopic = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_TRANSMISSIONS_KAFKA_TOPIC"); isPresent {
		cfg.TransmissionsKafkaTopic = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_EVENTS_KAFKA_TOPIC"); isPresent {
		cfg.EventsKafkaTopic = value
	}
	return nil
}

func validateConfig(cfg SolanaConfig) error {
	// Required config
	required := map[string]string{
		"SOLANA_NETWORK_NAME": cfg.NetworkName,
		"SOLANA_NETWORK_ID":   cfg.NetworkID,
		"SOLANA_CHAIN_ID":     cfg.ChainID,
	}
	if cfg.RunMode == "monitor" {
		required["SOLANA_RPC_ENDPOINT"] = cfg.RPCEndpoint
	}
	if cfg.RunMode == "ingestor" {
		required["SOLANA_WS_ENDPOINT"] = cfg.WSEndpoint
		required["SOLANA_STATE_KAFKA_TOPIC"] = cfg.StateKafkaTopic
		required["SOLANA_TRANSMISSIONS_KAFKA_TOPIC"] = cfg.TransmissionsKafkaTopic
		required["SOLANA_EVENTS_KAFKA_TOPIC"] = cfg.EventsKafkaTopic
	}
	for envVarName, currentValue := range required {
		if currentValue == "" {
			return fmt.Errorf("'%s' env var is required", envVarName)
		}
	}
	// Validate URLs.
	for envVarName, currentValue := range map[string]string{
		"SOLANA_RPC_ENDPOINT": cfg.RPCEndpoint,
		"SOLANA_WS_ENDPOINT":  cfg.WSEndpoint,
	} {
		if _, err := url.ParseRequestURI(currentValue); currentValue != "" && err != nil {
			return fmt.Errorf("%s='%s' is not a valid URL: %w", envVarName, currentValue, err)
		}
	}
	return nil
}

func applyDefaults(cfg *SolanaConfig) {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 2 * time.Second
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
}
