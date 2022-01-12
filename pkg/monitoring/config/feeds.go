package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gagliardetto/solana-go"
)

func populateFeeds(cfg *Config) error {
	if cfg.Feeds.FilePath == "" {
		return nil
	}
	contents, err := os.ReadFile(cfg.Feeds.FilePath)
	if err != nil {
		return fmt.Errorf("unable to read feeds file '%s': %w", cfg.Feeds.FilePath, err)
	}
	rawFeeds := []RawFeedConfig{}
	if err = json.Unmarshal(contents, &rawFeeds); err != nil {
		return fmt.Errorf("unable to unmarshal feeds config from file '%s': %w", cfg.Feeds.FilePath, err)
	}
	feeds, err := NewFeeds(rawFeeds)
	if err != nil {
		return err
	}
	cfg.Feeds.Feeds = feeds
	return nil
}

func NewFeeds(rawFeeds []RawFeedConfig) ([]Feed, error) {
	feeds := make([]Feed, len(rawFeeds))
	for i, rawFeed := range rawFeeds {
		contractAddress, err := solana.PublicKeyFromBase58(rawFeed.ContractAddressBase58)
		if err != nil {
			return nil, fmt.Errorf("failed to parse program id '%s' from JSON at index i=%d: %w", rawFeed.ContractAddressBase58, i, err)
		}
		transmissionsAccount, err := solana.PublicKeyFromBase58(rawFeed.TransmissionsAccountBase58)
		if err != nil {
			return nil, fmt.Errorf("failed to parse transmission account '%s' from JSON at index i=%d: %w", rawFeed.TransmissionsAccountBase58, i, err)
		}
		stateAccount, err := solana.PublicKeyFromBase58(rawFeed.StateAccountBase58)
		if err != nil {
			return nil, fmt.Errorf("failed to parse state account '%s' from JSON at index i=%d: %w", rawFeed.StateAccountBase58, i, err)
		}
		feeds[i] = Feed{
			rawFeed.FeedName,
			rawFeed.FeedPath,
			rawFeed.Symbol,
			rawFeed.Heartbeat,
			rawFeed.ContractType,
			rawFeed.ContractStatus,
			contractAddress,
			transmissionsAccount,
			stateAccount,
		}
	}
	return feeds, nil
}

// RawFeedConfig should only be used for deserializing responses from the RDD.
type RawFeedConfig struct {
	FeedName       string `json:"name,omitempty"`
	FeedPath       string `json:"path,omitempty"`
	Symbol         string `json:"symbol,omitempty"`
	Heartbeat      int64  `json:"heartbeat,omitempty"`
	ContractType   string `json:"contract_type,omitempty"`
	ContractStatus string `json:"status,omitempty"`

	ContractAddressBase58      string `json:"contract_address_base58,omitempty"`
	TransmissionsAccountBase58 string `json:"transmissions_account_base58,omitempty"`
	StateAccountBase58         string `json:"state_account_base58,omitempty"`
}
