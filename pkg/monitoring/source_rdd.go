package monitoring

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type FeedParser func(buf io.ReadCloser) ([]FeedConfig, error)

// rddSource produces a list of feeds to monitor.
type rddSource struct {
	rddURL     string
	httpClient *http.Client
	feedParser FeedParser
}

func NewRDDSource(
	rddURL string,
	feedParser FeedParser,
) Source {
	return &rddSource{
		rddURL,
		&http.Client{},
		feedParser,
	}
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
	feeds, err := r.feedParser(res.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse RDD data into an array of feed configurations: %w", err)
	}
	return feeds, nil
}

// FeedConfig is the interface for feed configurations extracted from the RDD.
type FeedConfig interface {
	GetName() string
	GetPath() string
	GetSymbol() string
	GetHeartbeatSec() int64
	GetContractType() string
	GetContractStatus() string
	// This functions as a feed identifier.
	GetContractAddress() string
	GetContractAddressBytes() []byte
	// Useful for mapping to kafka messages.
	ToMapping() map[string]interface{}
}
