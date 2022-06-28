package ingestor

import (
	"fmt"
	"net/url"
	"os"
	"time"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

type SolanaConfig struct {
	WSEndpoint  string
	NetworkName string
	NetworkID   string
	ChainID     string

	// Solana-specific kafka topics
	StatesKafkaTopic        string
	TransmissionsKafkaTopic string
	EventsKafkaTopic        string
	BlocksKafkaTopic        string
}

var _ relayMonitoring.ChainConfig = SolanaConfig{}

func (s SolanaConfig) GetRPCEndpoint() string         { return "" }
func (s SolanaConfig) GetNetworkName() string         { return s.NetworkName }
func (s SolanaConfig) GetNetworkID() string           { return s.NetworkID }
func (s SolanaConfig) GetChainID() string             { return s.ChainID }
func (s SolanaConfig) GetReadTimeout() time.Duration  { return time.Second }
func (s SolanaConfig) GetPollInterval() time.Duration { return time.Second }

func (s SolanaConfig) ToMapping() map[string]interface{} {
	return map[string]interface{}{}
}

func ParseSolanaConfig() (SolanaConfig, error) {
	cfg := SolanaConfig{}

	if err := parseEnvVars(&cfg); err != nil {
		return cfg, err
	}

	err := validateConfig(cfg)
	return cfg, err
}

func parseEnvVars(cfg *SolanaConfig) error {
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
	if value, isPresent := os.LookupEnv("SOLANA_STATES_KAFKA_TOPIC"); isPresent {
		cfg.StatesKafkaTopic = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_TRANSMISSIONS_KAFKA_TOPIC"); isPresent {
		cfg.TransmissionsKafkaTopic = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_EVENTS_KAFKA_TOPIC"); isPresent {
		cfg.EventsKafkaTopic = value
	}
	if value, isPresent := os.LookupEnv("SOLANA_BLOCKS_KAFKA_TOPIC"); isPresent {
		cfg.BlocksKafkaTopic = value
	}
	return nil
}

func validateConfig(cfg SolanaConfig) error {
	// Required config
	required := map[string]string{
		"SOLANA_NETWORK_NAME":              cfg.NetworkName,
		"SOLANA_NETWORK_ID":                cfg.NetworkID,
		"SOLANA_CHAIN_ID":                  cfg.ChainID,
		"SOLANA_WS_ENDPOINT":               cfg.WSEndpoint,
		"SOLANA_STATES_KAFKA_TOPIC":        cfg.StatesKafkaTopic,
		"SOLANA_TRANSMISSIONS_KAFKA_TOPIC": cfg.TransmissionsKafkaTopic,
		"SOLANA_EVENTS_KAFKA_TOPIC":        cfg.EventsKafkaTopic,
		"SOLANA_BLOCKS_KAFKA_TOPIC":        cfg.BlocksKafkaTopic,
	}
	for envVarName, currentValue := range required {
		if currentValue == "" {
			return fmt.Errorf("'%s' env var is required", envVarName)
		}
	}
	// Validate URLs.
	for envVarName, currentValue := range map[string]string{
		"SOLANA_WS_ENDPOINT": cfg.WSEndpoint,
	} {
		if _, err := url.ParseRequestURI(currentValue); currentValue != "" && err != nil {
			return fmt.Errorf("%s='%s' is not a valid URL: %w", envVarName, currentValue, err)
		}
	}
	return nil
}
