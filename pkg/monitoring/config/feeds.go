package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/gagliardetto/solana-go"
)

func populateFeeds(cfg *Config) error {
	feeds := []jsonFeedConfig{}
	if cfg.Feeds.URL != "" {
		rddCtx, cancel := context.WithTimeout(context.Background(), cfg.Feeds.RddReadTimeout)
		defer cancel()
		readFeedsReq, err := http.NewRequestWithContext(rddCtx, http.MethodGet, cfg.Feeds.URL, nil)
		if err != nil {
			return fmt.Errorf("unable to build a request to the RDD URL '%s': %w", cfg.Feeds.URL, err)
		}
		httpClient := &http.Client{}
		res, err := httpClient.Do(readFeedsReq)
		if err != nil {
			return fmt.Errorf("unable to fetch RDD data from URL '%s': %w", cfg.Feeds.URL, err)
		}
		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)
		if err := decoder.Decode(&feeds); err != nil {
			return fmt.Errorf("unable to unmarshal feeds config from RDD URL '%s': %w", cfg.Feeds.URL, err)
		}
	} else if cfg.Feeds.FilePath != "" {
		contents, err := os.ReadFile(cfg.Feeds.FilePath)
		if err != nil {
			return fmt.Errorf("unable to read feeds file '%s': %w", cfg.Feeds.FilePath, err)
		}
		if err = json.Unmarshal(contents, &feeds); err != nil {
			return fmt.Errorf("unable to unmarshal feeds config from file '%s': %w", cfg.Feeds.FilePath, err)
		}
	}

	cfg.Feeds.Feeds = make([]Feed, len(feeds))
	for i, feed := range feeds {
		contractAddress, err := solana.PublicKeyFromBase58(feed.ContractAddressBase58)
		if err != nil {
			return fmt.Errorf("failed to parse program id '%s' from JSON at index i=%d: %w", feed.ContractAddressBase58, i, err)
		}
		transmissionsAccount, err := solana.PublicKeyFromBase58(feed.TransmissionsAccountBase58)
		if err != nil {
			return fmt.Errorf("failed to parse transmission account '%s' from JSON at index i=%d: %w", feed.TransmissionsAccountBase58, i, err)
		}
		stateAccount, err := solana.PublicKeyFromBase58(feed.StateAccountBase58)
		if err != nil {
			return fmt.Errorf("failed to parse state account '%s' from JSON at index i=%d: %w", feed.StateAccountBase58, i, err)
		}
		cfg.Feeds.Feeds[i] = Feed{
			feed.FeedName,
			feed.FeedPath,
			feed.Symbol,
			feed.Heartbeat,
			feed.ContractType,
			feed.ContractStatus,
			contractAddress,
			transmissionsAccount,
			stateAccount,
		}
	}
	return nil
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
}
