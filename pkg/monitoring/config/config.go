package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

func Parse() (Config, error) {
	cfg := Config{}

	if err := parseEnvVars(&cfg); err != nil {
		return cfg, err
	}

	applyDefaults(&cfg)

	if err := validateConfig(cfg); err != nil {
		return cfg, err
	}

	err := populateFeeds(&cfg)
	return cfg, err
}

func parseEnvVars(cfg *Config) error {
	if value, isPresent := os.LookupEnv("SOLANA_RPC_ENDPOINT"); isPresent {
		cfg.Solana.RPCEndpoint = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_NETWORK_NAME"); isPresent {
		cfg.Solana.NetworkName = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_NETWORK_ID"); isPresent {
		cfg.Solana.NetworkID = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_CHAIN_ID"); isPresent {
		cfg.Solana.ChainID = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_READ_TIMEOUT"); isPresent {
		readTimeout, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("failed to parse env var SOLANA_READ_TIMEOUT, see https://pkg.go.dev/time#ParseDuration: %w", err)
		}
		cfg.Solana.ReadTimeout = readTimeout
	}
	if value, isPresent := os.LookupEnv("SOLANA_POLL_INTERVAL"); isPresent {
		pollInterval, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("failed to parse env var SOLANA_POLL_INTERVAL, see https://pkg.go.dev/time#ParseDuration: %w", err)
		}
		cfg.Solana.PollInterval = pollInterval
	}

	if value, isPresent := os.LookupEnv("KAFKA_BROKERS"); isPresent {
		cfg.Kafka.Brokers = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_CLIENT_ID"); isPresent {
		cfg.Kafka.ClientID = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SECURITY_PROTOCOL"); isPresent {
		cfg.Kafka.SecurityProtocol = value
	}

	if value, isPresent := os.LookupEnv("KAFKA_SASL_MECHANISM"); isPresent {
		cfg.Kafka.SaslMechanism = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SASL_USERNAME"); isPresent {
		cfg.Kafka.SaslUsername = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SASL_PASSWORD"); isPresent {
		cfg.Kafka.SaslPassword = value
	}

	if value, isPresent := os.LookupEnv("KAFKA_TRANSMISSION_TOPIC"); isPresent {
		cfg.Kafka.TransmissionTopic = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_CONFIG_SET_TOPIC"); isPresent {
		cfg.Kafka.ConfigSetTopic = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_CONFIG_SET_SIMPLIFIED_TOPIC"); isPresent {
		cfg.Kafka.ConfigSetSimplifiedTopic = value
	}

	if value, isPresent := os.LookupEnv("SCHEMA_REGISTRY_URL"); isPresent {
		cfg.SchemaRegistry.URL = value
	}
	if value, isPresent := os.LookupEnv("SCHEMA_REGISTRY_USERNAME"); isPresent {
		cfg.SchemaRegistry.Username = value
	}
	if value, isPresent := os.LookupEnv("SCHEMA_REGISTRY_PASSWORD"); isPresent {
		cfg.SchemaRegistry.Password = value
	}

	if value, isPresent := os.LookupEnv("FEEDS_URL"); isPresent {
		cfg.Feeds.URL = value
	}
	if value, isPresent := os.LookupEnv("FEEDS_FILE_PATH"); isPresent {
		cfg.Feeds.FilePath = value
	}
	if value, isPresent := os.LookupEnv("FEEDS_RDD_READ_TIMEOUT"); isPresent {
		readTimeout, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("failed to parse env var FEEDS_RDD_READ_TIMEOUT, see https://pkg.go.dev/time#ParseDuration: %w", err)
		}
		cfg.Feeds.RddReadTimeout = readTimeout
	}
	if value, isPresent := os.LookupEnv("FEEDS_RDD_POLL_INTERVAL"); isPresent {
		pollInterval, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("failed to parse env var FEEDS_RDD_POLL_INTERVAL, see https://pkg.go.dev/time#ParseDuration: %w", err)
		}
		cfg.Feeds.RddPollInterval = pollInterval
	}

	if value, isPresent := os.LookupEnv("HTTP_ADDRESS"); isPresent {
		cfg.Http.Address = value
	}

	if value, isPresent := os.LookupEnv("FEATURE_TEST_MODE"); isPresent {
		isTestMode, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("failed to parse boolean env var '%s'. See https://pkg.go.dev/strconv#ParseBool", "FEATURE_TEST_MODE")
		}
		cfg.Feature.TestMode = isTestMode
	}

	return nil
}

func validateConfig(cfg Config) error {
	// Required config
	for envVarName, currentValue := range map[string]string{
		"SOLANA_RPC_ENDPOINT": cfg.Solana.RPCEndpoint,
		"SOLANA_NETWORK_NAME": cfg.Solana.NetworkName,
		"SOLANA_NETWORK_ID":   cfg.Solana.NetworkID,
		"SOLANA_CHAIN_ID":     cfg.Solana.ChainID,

		"KAFKA_BROKERS":           cfg.Kafka.Brokers,
		"KAFKA_CLIENT_ID":         cfg.Kafka.ClientID,
		"KAFKA_SECURITY_PROTOCOL": cfg.Kafka.SecurityProtocol,
		"KAFKA_SASL_MECHANISM":    cfg.Kafka.SaslMechanism,

		"KAFKA_CONFIG_SET_TOPIC":            cfg.Kafka.ConfigSetTopic,
		"KAFKA_TRANSMISSION_TOPIC":          cfg.Kafka.TransmissionTopic,
		"KAFKA_CONFIG_SET_SIMPLIFIED_TOPIC": cfg.Kafka.ConfigSetSimplifiedTopic,

		"SCHEMA_REGISTRY_URL": cfg.SchemaRegistry.URL,

		"HTTP_ADDRESS": cfg.Http.Address,
	} {
		if currentValue == "" {
			return fmt.Errorf("'%s' env var is required", envVarName)
		}
	}
	// Validate feeds.
	if cfg.Feeds.URL == "" && cfg.Feeds.FilePath == "" {
		return fmt.Errorf("must set one of 'FEEDS_URL' or 'FEEDS_FILE_PATH'")
	}
	if cfg.Feeds.URL != "" && cfg.Feeds.FilePath != "" {
		return fmt.Errorf("can't set both 'FEEDS_URL' and 'FEEDS_FILE_PATH'. Only one allowed")
	}
	// Validate URLs.
	for envVarName, currentValue := range map[string]string{
		"SOLANA_RPC_ENDPOINT": cfg.Solana.RPCEndpoint,
		"SCHEMA_REGISTRY_URL": cfg.SchemaRegistry.URL,
		"FEEDS_URL":           cfg.Feeds.URL,
	} {
		if currentValue == "" {
			continue
		}
		if _, err := url.ParseRequestURI(currentValue); err != nil {
			return fmt.Errorf("%s='%s' is not a valid URL: %w", envVarName, currentValue, err)
		}
	}
	return nil
}
