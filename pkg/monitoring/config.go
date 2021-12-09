package monitoring

import (
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
}

type SolanaConfig struct {
	RPCEndpoint string `json:"rpc_endpoint,omitempty"`
	NetworkName string `json:"network_name,omitempty"`
	NetworkID   string `json:"network_id,omitempty"`
	ChainID     string `json:"chain_id,omitempty"`
}

type KafkaConfig struct {
	Brokers          string `json:"brokers,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	SecurityProtocol string `json:"security_protocol,omitempty"`
	SaslMechanism    string `json:"sasl_mechanism,omitempty"`
	SaslUsername     string `json:"sasl_username,omitempty"`
	SaslPassword     string `json:"sasl_password,omitempty"`
	Topic            string `json:"topic,omitempty"`
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
)

// ParseConfig populates a configuration object from various sources:
// - most params are passed as flags to the binary.
// - username and passwords can be overriden by environment variables.
// - feeds configuration can be passed by an RDD url or a local file (useful for testing).
// ParseConfig also validates and parses some of these inputs and returns an error for the first input that is found incorrect.
func ParseConfig() (Config, error) {

	cfg := Config{}
	flag.StringVar(&cfg.Solana.RPCEndpoint, "solana.rpc_endpoint", "", "")
	flag.StringVar(&cfg.Solana.NetworkName, "solana.network_name", "", "")
	flag.StringVar(&cfg.Solana.NetworkID, "solana.network_id", "", "")
	flag.StringVar(&cfg.Solana.ChainID, "solana.chain_id", "", "")

	flag.StringVar(&cfg.Kafka.Topic, "kafka.topic", "", "solana-mainnet")
	flag.StringVar(&cfg.Kafka.Brokers, "kafka.brokers", "", "")
	flag.StringVar(&cfg.Kafka.ClientID, "kafka.client_id", "", "")
	flag.StringVar(&cfg.Kafka.SecurityProtocol, "kafka.security_protocol", "", "")
	flag.StringVar(&cfg.Kafka.SaslMechanism, "kafka.sasl_mechanism", "", "")
	flag.StringVar(&cfg.Kafka.SaslUsername, "kafka.sasl_username", "", "")
	flag.StringVar(&cfg.Kafka.SaslPassword, "kafka.sasl_password", "", "")

	flag.StringVar(&cfg.SchemaRegistry.URL, "schema_registry.url", "", "")
	flag.StringVar(&cfg.SchemaRegistry.Username, "schema_registry.username", "", "")
	flag.StringVar(&cfg.SchemaRegistry.Password, "schema_registry.password", "", "")

	var feedsFilePath string
	var feedsRDDURL string
	flag.StringVar(&feedsFilePath, "feeds.file_path", "", "")
	flag.StringVar(&feedsRDDURL, "feeds.rdd_url", "", "")

	flag.StringVar(&cfg.Http.Address, "http.address", "", "")

	flag.Parse()

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

	if value, isPresent := os.LookupEnv("KAFKA_TOPIC"); isPresent {
		cfg.Kafka.Topic = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SASL_USERNAME"); isPresent {
		cfg.Kafka.SaslUsername = value
	}
	if value, isPresent := os.LookupEnv("KAFKA_SASL_PASSWORD"); isPresent {
		cfg.Kafka.SaslPassword = value
	}
	if value, isPresent := os.LookupEnv("SCHEMA_REGISTRY_USERNAME"); isPresent {
		cfg.SchemaRegistry.Username = value
	}
	if value, isPresent := os.LookupEnv("SCHEMA_REGISTRY_PASSWORD"); isPresent {
		cfg.SchemaRegistry.Password = value
	}

	var feeds = []jsonFeedConfig{}
	if feedsFilePath == "" && feedsRDDURL == "" {
		return cfg, fmt.Errorf("feeds configuration missing, either '-feeds.file_path' or '-feeds.rdd_url' must be set")
	} else if feedsRDDURL != "" {
		res, err := http.Get(feedsRDDURL)
		if err != nil {
			return cfg, fmt.Errorf("unable to contact RDD URL %s: %w", feedsRDDURL, err)
		}
		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)
		if err := decoder.Decode(&feeds); err != nil {
			return cfg, fmt.Errorf("unable to unmarshal feeds config from RDD URL %s: %w", feedsRDDURL, err)
		}
	} else if feedsFilePath != "" {
		contents, err := os.ReadFile(feedsFilePath)
		if err != nil {
			return cfg, fmt.Errorf("unable to read feeds file %s: %w", feedsFilePath, err)
		}
		if err = json.Unmarshal(contents, &feeds); err != nil {
			return cfg, fmt.Errorf("unable to unmarshal feeds config from file %s: %w", feedsFilePath, err)
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
			return cfg, fmt.Errorf("failed to parse state account '%s' from JSON at index i=%d: %w", feed.TransmissionsAccountBase58, i, err)
		}
		if feed.PollInterval.Nanoseconds() == 0 {
			feed.PollInterval = DefaultPollInterval
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
			feed.PollInterval,
		}
	}

	return cfg, nil
}

type jsonFeedConfig struct {
	FeedName       string `json:"name,omitempty"`
	FeedPath       string `json:"path,omitempty"`
	Symbol         string `json:"symbol,omitempty"`
	Heartbeat      int64  `json:"heartbeat,omitempty"`
	ContractType   string `json:"contractType,omitempty"`
	ContractStatus string `json:"status,omitempty"`

	ContractAddressBase58      string `json:"contractAddress,omitempty"`
	TransmissionsAccountBase58 string `json:"transmissionsAccount,omitempty"`
	StateAccountBase58         string `json:"stateAccount,omitempty"`

	PollInterval time.Duration `json:"poll_interval,omitempty"`
}
