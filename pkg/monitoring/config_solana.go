package monitoring

import (
	"fmt"
	"net/url"
	"os"
	"time"
)

type SolanaConfig struct {
	RPCEndpoint  string
	NetworkName  string
	NetworkID    string
	ChainID      string
	ReadTimeout  time.Duration
	PollInterval time.Duration
}

func ParseSolanaConfig() (SolanaConfig, error) {
	cfg := SolanaConfig{}

	if err := parseEnvVars(&cfg); err != nil {
		return cfg, err
	}

	applyDefaults(&cfg)

	err := validateConfig(cfg)
	return cfg, err
}

func parseEnvVars(cfg *SolanaConfig) error {
	if value, isPresent := os.LookupEnv("SOLANA_RPC_ENDPOINT"); isPresent {
		cfg.RPCEndpoint = value
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
	return nil
}

func validateConfig(cfg SolanaConfig) error {
	// Required config
	for envVarName, currentValue := range map[string]string{
		"SOLANA_RPC_ENDPOINT": cfg.RPCEndpoint,
		"SOLANA_NETWORK_NAME": cfg.NetworkName,
		"SOLANA_NETWORK_ID":   cfg.NetworkID,
		"SOLANA_CHAIN_ID":     cfg.ChainID,
	} {
		if currentValue == "" {
			return fmt.Errorf("'%s' env var is required", envVarName)
		}
	}
	// Validate URLs.
	for envVarName, currentValue := range map[string]string{
		"SOLANA_RPC_ENDPOINT": cfg.RPCEndpoint,
	} {
		if _, err := url.ParseRequestURI(currentValue); err != nil {
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
