package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
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
	rawFeeds := []config.RawFeedConfig{}
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&rawFeeds); err != nil {
		return nil, fmt.Errorf("unable to unmarshal feeds config data: %w", err)
	}
	return config.NewFeeds(rawFeeds)
}
