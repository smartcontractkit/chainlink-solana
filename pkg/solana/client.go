package solana

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/sync/singleflight"
)

// Client contains the rpc and requestGroup for a given network
type Client struct {
	rpc             *rpc.Client
	skipPreflight   bool // to enable or disable preflight checks
	commitment      rpc.CommitmentType
	txTimeout       time.Duration
	pollingInterval time.Duration
	contextDuration time.Duration

	// provides a duplicate function call suppression mechanism
	requestGroup *singleflight.Group
}

// NewClient will bundle the RPC and requestGroup together as a network Client
func NewClient(spec OCR2Spec, logger Logger) *Client {
	client := &Client{
		rpc:           rpc.New(spec.NodeEndpointHTTP),
		skipPreflight: !spec.UsePreflight,
		requestGroup:  &singleflight.Group{},
	}

	// parse commitment level (defaults to confirmed)
	switch spec.Commitment {
	case "processed":
		client.commitment = rpc.CommitmentProcessed
	case "finalized":
		client.commitment = rpc.CommitmentFinalized
	default:
		client.commitment = rpc.CommitmentConfirmed
	}

	// parse poll interval, if errors: use 1 second default
	pollInterval, err := time.ParseDuration(spec.PollingInterval)
	if err != nil {
		logger.Warnf("could not parse polling interval ('%s') using default (%s)", spec.PollingInterval, defaultPollInterval)
		pollInterval = defaultPollInterval
	}

	// parse context length, if errors, use 2x poll interval
	ctxInterval, err := time.ParseDuration(spec.PollingCtxTimeout)
	if err != nil {
		logger.Warnf("could not parse polling context duration ('%s') using default 2x polling interval (%s)", spec.PollingCtxTimeout, 2*pollInterval)
		ctxInterval = 2 * pollInterval
	}

	// parse tx context, if errors use defaultStaleTimeout
	txTimeout, err := time.ParseDuration(spec.TxTimeout)
	if err != nil {
		logger.Warnf("could not parse tx context duration ('%s') using default (%s)", spec.TxTimeout, defaultStaleTimeout)
		txTimeout = defaultStaleTimeout
	}

	client.pollingInterval = pollInterval
	client.contextDuration = ctxInterval
	client.txTimeout = txTimeout

	// log client configuration
	logger.Debugf("NewClient configuration: %+v", client)

	return client
}

// GetBlockHeight returns the height of the most recent processed slot in the chain, coalescing requests.
// GetBlockHeight is a required method for libocr, however this implementation uses solana slots to match the onchain implementation
func (c Client) GetBlockHeight(ctx context.Context, commitment rpc.CommitmentType) (blockHeight uint64, err error) {
	// do single flight request
	v, err, _ := c.requestGroup.Do("GetSlotHeight", func() (interface{}, error) {
		return c.rpc.GetSlot(ctx, commitment)
	})

	if err != nil {
		return 0, err
	}

	return v.(uint64), nil
}
