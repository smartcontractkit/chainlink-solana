package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gagliardetto/solana-go"
)

// rddSource produces a list of feeds to monitor.
type rddSource struct {
	rddURL     string
	httpClient *http.Client
}

func NewRDDSource(
	rddURL string,
) Source {
	return &rddSource{
		rddURL,
		&http.Client{},
	}
}

func (r *rddSource) Name() string {
	return "rdd"
}

func (r *rddSource) Fetch(ctx context.Context) (interface{}, error) {
	readFeedsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.rddURL, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to build a request to the RDD: %w", err)
	}
	res, err := r.httpClient.Do(readFeedsReq)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch RDD data: %w", err)
	}
	defer res.Body.Close()
	rawFeeds := []rawFeedConfig{}
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&rawFeeds); err != nil {
		return nil, fmt.Errorf("unable to unmarshal feeds config data: %w", err)
	}
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

type rawFeedConfig struct {
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

type Feed struct {
	// Data extracted from the RDD
	FeedName       string
	FeedPath       string
	Symbol         string
	HeartbeatSec   int64
	ContractType   string
	ContractStatus string

	// Equivalent to ProgramID in Solana
	ContractAddress      solana.PublicKey
	TransmissionsAccount solana.PublicKey
	StateAccount         solana.PublicKey
}
