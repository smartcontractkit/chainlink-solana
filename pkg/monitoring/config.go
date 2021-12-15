package monitoring

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gagliardetto/solana-go"
)

type Config struct {
	Solana         SolanaConfig         `json:"solana,omitempty"`
	Kafka          KafkaConfig          `json:"kafka,omitempty"`
	SchemaRegistry SchemaRegistryConfig `json:"schema_registry,omitempty"`
	Feeds          []FeedConfig         `json:"feeds,omitempty"`
	Http           HttpConfig           `json:"http,omitempty"`
	FeedsRddURL    string               `json:"feedsRddUrl,omitempty"`
	FeedsFilePath  string               `json:"feedsFilePath,omitempty"`
}

type SolanaConfig struct {
	RPCEndpoint string `json:"rpc_endpoint,omitempty"`
	NetworkName string `json:"network_name,omitempty"`
	NetworkID   string `json:"network_id,omitempty"`
	ChainID     string `json:"chain_id,omitempty"`
}

type KafkaConfig struct {
	Brokers                  string `json:"brokers,omitempty"`
	ClientID                 string `json:"client_id,omitempty"`
	SecurityProtocol         string `json:"security_protocol,omitempty"`
	SaslMechanism            string `json:"sasl_mechanism,omitempty"`
	SaslUsername             string `json:"sasl_username,omitempty"`
	SaslPassword             string `json:"sasl_password,omitempty"`
	TransmissionTopic        string `json:"kafka_transmission_topic,omitempty"`
	ConfigSetTopic           string `json:"config_set_topic,omitempty"`
	ConfigSetSimplifiedTopic string `json:"config_set_simplified_topic,omitempty"`
}

type SchemaRegistryConfig struct {
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type FeedConfig struct {
	// Data extracted from the RDD
	FeedName       string `json:"feed_name,omitempty"`
	FeedPath       string `json:"feed_path,omitempty"`
	Symbol         string `json:"symbol,omitempty"`
	HeartbeatSec   int64  `json:"heartbeat,omitempty"`
	ContractType   string `json:"contract_type,omitempty"`
	ContractStatus string `json:"contract_status,omitempty"`

	// Equivalent to ProgramID in Solana
	ContractAddress      solana.PublicKey `json:"contract_address,omitempty"`
	TransmissionsAccount solana.PublicKey `json:"transmissions_account,omitempty"`
	StateAccount         solana.PublicKey `json:"state_account,omitempty"`

	PollInterval time.Duration `json:"poll_interval,omitempty"`
}

type HttpConfig struct {
	Address string `json:"address,omitempty"`
}

const (
	DefaultPollInterval = 5 * time.Second

	rddHttpCallTimeout = 5 * time.Second
)

// ParseConfig populates a configuration object from various sources:
// - most params are passed as flags to the binary.
// - username and passwords can be overriden by environment variables.
// - feeds configuration can be passed by an RDD url or a local file (useful for testing).
// ParseConfig also validates and parses some of these inputs and returns an error for the first input that is found incorrect.
func ParseConfig(ctx context.Context) (Config, error) {

	cfg := Config{}
	flag.StringVar(&cfg.Solana.RPCEndpoint, "solana.rpc_endpoint", "", "")
	flag.StringVar(&cfg.Solana.NetworkName, "solana.network_name", "", "")
	flag.StringVar(&cfg.Solana.NetworkID, "solana.network_id", "", "")
	flag.StringVar(&cfg.Solana.ChainID, "solana.chain_id", "", "")

	flag.StringVar(&cfg.Kafka.ConfigSetTopic, "kafka.config_set_topic", "", "")
	flag.StringVar(&cfg.Kafka.ConfigSetSimplifiedTopic, "kafka.config_set_simplified_topic", "", "")
	flag.StringVar(&cfg.Kafka.TransmissionTopic, "kafka.transmission_topic", "", "")
	flag.StringVar(&cfg.Kafka.Brokers, "kafka.brokers", "", "")
	flag.StringVar(&cfg.Kafka.ClientID, "kafka.client_id", "", "")
	flag.StringVar(&cfg.Kafka.SecurityProtocol, "kafka.security_protocol", "", "")
	flag.StringVar(&cfg.Kafka.SaslMechanism, "kafka.sasl_mechanism", "", "")
	flag.StringVar(&cfg.Kafka.SaslUsername, "kafka.sasl_username", "", "")
	flag.StringVar(&cfg.Kafka.SaslPassword, "kafka.sasl_password", "", "")

	flag.StringVar(&cfg.SchemaRegistry.URL, "schema_registry.url", "", "")
	flag.StringVar(&cfg.SchemaRegistry.Username, "schema_registry.username", "", "")
	flag.StringVar(&cfg.SchemaRegistry.Password, "schema_registry.password", "", "")

	flag.StringVar(&cfg.FeedsFilePath, "feeds.file_path", "", "")
	flag.StringVar(&cfg.FeedsRddURL, "feeds.rdd_url", "", "")

	flag.StringVar(&cfg.Http.Address, "http.address", "", "")

	flag.Parse()

	parseEnvVars(&cfg)

	for flagName, value := range map[string]string{
		"-solana.rpc_endpoint": cfg.Solana.RPCEndpoint,

		"-kafka.brokers":           cfg.Kafka.Brokers,
		"-kafka.client_id":         cfg.Kafka.ClientID,
		"-kafka.security_protocol": cfg.Kafka.SecurityProtocol,

		"-schema_registry.url": cfg.SchemaRegistry.URL,

		"-http.address": cfg.Http.Address,
	} {
		if value == "" {
			return cfg, fmt.Errorf("flag '%s' is required", flagName)
		}
	}

	var feeds = []jsonFeedConfig{}
	if cfg.FeedsFilePath == "" && cfg.FeedsRddURL == "" {
		return cfg, fmt.Errorf("feeds configuration missing, either '-feeds.file_path' or '-feeds.rdd_url' must be set")
	} else if cfg.FeedsRddURL != "" {
		rddCtx, cancel := context.WithTimeout(ctx, rddHttpCallTimeout)
		defer cancel()
		readFeedsReq, err := http.NewRequestWithContext(rddCtx, http.MethodGet, cfg.FeedsRddURL, nil)
		if err != nil {
			return cfg, fmt.Errorf("unable to build a request to the RDD URL '%s': %w", cfg.FeedsRddURL, err)
		}
		httpClient := &http.Client{}
		res, err := httpClient.Do(readFeedsReq)
		if err != nil {
			return cfg, fmt.Errorf("unable to fetch RDD data from URL '%s': %w", cfg.FeedsRddURL, err)
		}
		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)
		if err := decoder.Decode(&feeds); err != nil {
			return cfg, fmt.Errorf("unable to unmarshal feeds config from RDD URL '%s': %w", cfg.FeedsRddURL, err)
		}
	} else if cfg.FeedsFilePath != "" {
		contents, err := os.ReadFile(cfg.FeedsFilePath)
		if err != nil {
			return cfg, fmt.Errorf("unable to read feeds file '%s': %w", cfg.FeedsFilePath, err)
		}
		if err = json.Unmarshal(contents, &feeds); err != nil {
			return cfg, fmt.Errorf("unable to unmarshal feeds config from file '%s': %w", cfg.FeedsFilePath, err)
		}
	}

	cfg.Feeds = make([]FeedConfig, len(feeds))
	for i, feed := range feeds {
		contractAddress, err := solana.PublicKeyFromBase58(feed.ContractAddressBase58)
		if err != nil {
			return cfg, fmt.Errorf("failed to parse program id '%s' from JSON at index i=%d: %w", feed.ContractAddressBase58, i, err)
		}
		transmissionsAccount, err := solana.PublicKeyFromBase58(feed.TransmissionsAccountBase58)
		if err != nil {
			return cfg, fmt.Errorf("failed to parse transmission account '%s' from JSON at index i=%d: %w", feed.TransmissionsAccountBase58, i, err)
		}
		stateAccount, err := solana.PublicKeyFromBase58(feed.StateAccountBase58)
		if err != nil {
			return cfg, fmt.Errorf("failed to parse state account '%s' from JSON at index i=%d: %w", feed.StateAccountBase58, i, err)
		}
		pollInterval := DefaultPollInterval
		if feed.PollIntervalMilliseconds != 0 {
			pollInterval = time.Duration(feed.PollIntervalMilliseconds) * time.Millisecond
		}
		cfg.Feeds[i] = FeedConfig{
			feed.FeedName,
			feed.FeedPath,
			feed.Symbol,
			feed.Heartbeat,
			feed.ContractType,
			feed.ContractStatus,
			contractAddress,
			transmissionsAccount,
			stateAccount,
			pollInterval,
		}
	}

	return cfg, nil
}

func parseEnvVars(cfg *Config) {
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
	if value, isPresent := os.LookupEnv("KAFKA_TRANSMISSION_TOPIC"); isPresent {
		cfg.Kafka.TransmissionTopic = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_CONFIG_SET_TOPIC"); isPresent {
		cfg.Kafka.ConfigSetTopic = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_CONFIG_SET_SIMPLIFIED_TOPIC"); isPresent {
		cfg.Kafka.ConfigSetSimplifiedTopic = value
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
		cfg.Kafka.SaslUsername = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SASL_USERNAME"); isPresent {
		cfg.Kafka.SaslUsername = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SASL_PASSWORD"); isPresent {
		cfg.Kafka.SaslPassword = value
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

	if value, isPresent := os.LookupEnv("HTTP_ADDRESS"); isPresent {
		cfg.Http.Address = value
	}

	if value, isPresent := os.LookupEnv("FEEDS_FILE_PATH"); isPresent {
		cfg.FeedsFilePath = value
	}
	if value, isPresent := os.LookupEnv("FEEDS_URL"); isPresent {
		cfg.FeedsRddURL = value
	}
}

type jsonFeedConfig struct {
	FeedName       string `json:"name,omitempty"`
	FeedPath       string `json:"path,omitempty"`
	Symbol         string `json:"symbol,omitempty"`
	Heartbeat      int64  `json:"heartbeat,omitempty"`
	ContractType   string `json:"contract_type,omitempty"`
	ContractStatus string `json:"status,omitempty"`

	ContractAddressBase58      string `json:"contract_address_base58,omitempty"`
	TransmissionsAccountBase58 string `json:"transmissions_account_base58,omitempty"`
	StateAccountBase58         string `json:"state_account_base58,omitempty"`

	PollIntervalMilliseconds int64 `json:"poll_interval_milliseconds,omitempty"`
}
