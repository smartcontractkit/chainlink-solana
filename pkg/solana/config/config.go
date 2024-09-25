package config

import (
	"errors"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
)

// Global solana defaults.
var defaultConfigSet = Chain{
	BalancePollPeriod:   config.MustNewDuration(5 * time.Second),        // poll period for balance monitoring
	ConfirmPollPeriod:   config.MustNewDuration(500 * time.Millisecond), // polling for tx confirmation
	OCR2CachePollPeriod: config.MustNewDuration(time.Second),            // cache polling rate
	OCR2CacheTTL:        config.MustNewDuration(time.Minute),            // stale cache deadline
	TxTimeout:           config.MustNewDuration(time.Minute),            // timeout for send tx method in client
	TxRetryTimeout:      config.MustNewDuration(10 * time.Second),       // duration for tx rebroadcasting to RPC node
	TxConfirmTimeout:    config.MustNewDuration(30 * time.Second),       // duration before discarding tx as unconfirmed
	SkipPreflight:       ptr(true),                                      // to enable or disable preflight checks
	Commitment:          ptr(string(rpc.CommitmentConfirmed)),
	MaxRetries:          ptr(int64(0)), // max number of retries (default = 0). when config.MaxRetries < 0), interpreted as MaxRetries = nil and rpc node will do a reasonable number of retries

	// fee estimator
	FeeEstimatorMode:        ptr("fixed"),
	ComputeUnitPriceMax:     ptr(uint64(1_000)),
	ComputeUnitPriceMin:     ptr(uint64(0)),
	ComputeUnitPriceDefault: ptr(uint64(0)),
	FeeBumpPeriod:           config.MustNewDuration(3 * time.Second), // set to 0 to disable fee bumping
	BlockHistoryPollPeriod:  config.MustNewDuration(5 * time.Second),
	ComputeUnitLimitDefault: ptr(uint32(200_000)),
}

//go:generate mockery --name Config --output ./mocks/ --case=underscore --filename config.go
type Config interface {
	BalancePollPeriod() time.Duration
	ConfirmPollPeriod() time.Duration
	OCR2CachePollPeriod() time.Duration
	OCR2CacheTTL() time.Duration
	TxTimeout() time.Duration
	TxRetryTimeout() time.Duration
	TxConfirmTimeout() time.Duration
	SkipPreflight() bool
	Commitment() rpc.CommitmentType
	MaxRetries() *uint

	// fee estimator
	FeeEstimatorMode() string
	ComputeUnitPriceMax() uint64
	ComputeUnitPriceMin() uint64
	ComputeUnitPriceDefault() uint64
	FeeBumpPeriod() time.Duration
	BlockHistoryPollPeriod() time.Duration
	ComputeUnitLimitDefault() uint32
}

type Chain struct {
	BalancePollPeriod       *config.Duration
	ConfirmPollPeriod       *config.Duration
	OCR2CachePollPeriod     *config.Duration
	OCR2CacheTTL            *config.Duration
	TxTimeout               *config.Duration
	TxRetryTimeout          *config.Duration
	TxConfirmTimeout        *config.Duration
	SkipPreflight           *bool
	Commitment              *string
	MaxRetries              *int64
	FeeEstimatorMode        *string
	ComputeUnitPriceMax     *uint64
	ComputeUnitPriceMin     *uint64
	ComputeUnitPriceDefault *uint64
	FeeBumpPeriod           *config.Duration
	BlockHistoryPollPeriod  *config.Duration
	ComputeUnitLimitDefault *uint32
}

func (c *Chain) SetDefaults() {
	if c.BalancePollPeriod == nil {
		c.BalancePollPeriod = defaultConfigSet.BalancePollPeriod
	}
	if c.ConfirmPollPeriod == nil {
		c.ConfirmPollPeriod = defaultConfigSet.ConfirmPollPeriod
	}
	if c.OCR2CachePollPeriod == nil {
		c.OCR2CachePollPeriod = defaultConfigSet.OCR2CachePollPeriod
	}
	if c.OCR2CacheTTL == nil {
		c.OCR2CacheTTL = defaultConfigSet.OCR2CacheTTL
	}
	if c.TxTimeout == nil {
		c.TxTimeout = defaultConfigSet.TxTimeout
	}
	if c.TxRetryTimeout == nil {
		c.TxRetryTimeout = defaultConfigSet.TxRetryTimeout
	}
	if c.TxConfirmTimeout == nil {
		c.TxConfirmTimeout = defaultConfigSet.TxConfirmTimeout
	}
	if c.SkipPreflight == nil {
		c.SkipPreflight = defaultConfigSet.SkipPreflight
	}
	if c.Commitment == nil {
		c.Commitment = defaultConfigSet.Commitment
	}
	if c.MaxRetries == nil {
		c.MaxRetries = defaultConfigSet.MaxRetries
	}
	if c.FeeEstimatorMode == nil {
		c.FeeEstimatorMode = defaultConfigSet.FeeEstimatorMode
	}
	if c.ComputeUnitPriceMax == nil {
		c.ComputeUnitPriceMax = defaultConfigSet.ComputeUnitPriceMax
	}
	if c.ComputeUnitPriceMin == nil {
		c.ComputeUnitPriceMin = defaultConfigSet.ComputeUnitPriceMin
	}
	if c.ComputeUnitPriceDefault == nil {
		c.ComputeUnitPriceDefault = defaultConfigSet.ComputeUnitPriceDefault
	}
	if c.FeeBumpPeriod == nil {
		c.FeeBumpPeriod = defaultConfigSet.FeeBumpPeriod
	}
	if c.BlockHistoryPollPeriod == nil {
		c.BlockHistoryPollPeriod = defaultConfigSet.BlockHistoryPollPeriod
	}
	if c.ComputeUnitLimitDefault == nil {
		c.ComputeUnitLimitDefault = defaultConfigSet.ComputeUnitLimitDefault
	}
}

type Node struct {
	Name     *string
	URL      *config.URL
	SendOnly bool
}

func (n *Node) ValidateConfig() (err error) {
	if n.Name == nil {
		err = errors.Join(err, config.ErrMissing{Name: "Name", Msg: "required for all nodes"})
	} else if *n.Name == "" {
		err = errors.Join(err, config.ErrEmpty{Name: "Name", Msg: "required for all nodes"})
	}
	if n.URL == nil {
		err = errors.Join(err, config.ErrMissing{Name: "URL", Msg: "required for all nodes"})
	}
	return
}

func ptr[T any](t T) *T {
	return &t
}
