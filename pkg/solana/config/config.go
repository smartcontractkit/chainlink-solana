package config

import (
	"strings"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/multierr"

	relaycfg "github.com/smartcontractkit/chainlink-relay/pkg/config"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"

	htrktypes "github.com/smartcontractkit/chainlink-solana/pkg/common/headtracker/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
)

// Global solana defaults.
var defaultConfigSet = configSet{
	BalancePollPeriod:   5 * time.Second,        // poll period for balance monitoring
	ConfirmPollPeriod:   500 * time.Millisecond, // polling for tx confirmation
	OCR2CachePollPeriod: time.Second,            // cache polling rate
	OCR2CacheTTL:        time.Minute,            // stale cache deadline
	TxTimeout:           time.Minute,            // timeout for send tx method in client
	TxRetryTimeout:      10 * time.Second,       // duration for tx rebroadcasting to RPC node
	TxConfirmTimeout:    30 * time.Second,       // duration before discarding tx as unconfirmed
	SkipPreflight:       true,                   // to enable or disable preflight checks
	Commitment:          rpc.CommitmentConfirmed,
	MaxRetries:          new(uint), // max number of retries, when nil - rpc node will do a reasonable number of retries

	// fee estimator
	FeeEstimatorMode:        "fixed",
	ComputeUnitPriceMax:     1_000,
	ComputeUnitPriceMin:     0,
	ComputeUnitPriceDefault: 0,
	FeeBumpPeriod:           3 * time.Second,

	// headtracker
	BlockEmissionIdleWarningThreshold: 30 * time.Second,
	FinalityDepth:                     50,
	HeadTrackerHistoryDepth:           100,
	HeadTrackerMaxBufferSize:          3,
	HeadTrackerSamplingInterval:       1 * time.Second,
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

	// headtracker
	BlockEmissionIdleWarningThreshold() time.Duration
	FinalityDepth() uint32
	HeadTrackerHistoryDepth() uint32
	HeadTrackerMaxBufferSize() uint32
	HeadTrackerSamplingInterval() time.Duration
}

// opt: remove
type configSet struct {
	BalancePollPeriod   time.Duration
	ConfirmPollPeriod   time.Duration
	OCR2CachePollPeriod time.Duration
	OCR2CacheTTL        time.Duration
	TxTimeout           time.Duration
	TxRetryTimeout      time.Duration
	TxConfirmTimeout    time.Duration
	SkipPreflight       bool
	Commitment          rpc.CommitmentType
	MaxRetries          *uint

	FeeEstimatorMode        string
	ComputeUnitPriceMax     uint64
	ComputeUnitPriceMin     uint64
	ComputeUnitPriceDefault uint64
	FeeBumpPeriod           time.Duration

	BlockEmissionIdleWarningThreshold time.Duration
	FinalityDepth                     uint32
	HeadTrackerHistoryDepth           uint32
	HeadTrackerMaxBufferSize          uint32
	HeadTrackerSamplingInterval       time.Duration
}

var _ Config = (*config)(nil)
var _ htrktypes.Config = (*config)(nil)

// Deprecated
type config struct {
	defaults configSet
	chain    db.ChainCfg
	lggr     logger.Logger
}

// NewConfig returns a Config with defaults overridden by dbcfg.
// Deprecated
func NewConfig(dbcfg db.ChainCfg, lggr logger.Logger) *config {
	return &config{
		defaults: defaultConfigSet,
		chain:    dbcfg,
		lggr:     lggr,
	}
}

func (c *config) BalancePollPeriod() time.Duration {
	ch := c.chain.BalancePollPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.BalancePollPeriod
}

func (c *config) ConfirmPollPeriod() time.Duration {
	ch := c.chain.ConfirmPollPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.ConfirmPollPeriod
}

func (c *config) OCR2CachePollPeriod() time.Duration {
	ch := c.chain.OCR2CachePollPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.OCR2CachePollPeriod
}

func (c *config) OCR2CacheTTL() time.Duration {
	ch := c.chain.OCR2CacheTTL
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.OCR2CacheTTL
}

func (c *config) TxTimeout() time.Duration {
	ch := c.chain.TxTimeout
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxTimeout
}

func (c *config) TxRetryTimeout() time.Duration {
	ch := c.chain.TxRetryTimeout
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxRetryTimeout
}

func (c *config) TxConfirmTimeout() time.Duration {
	ch := c.chain.TxConfirmTimeout
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxConfirmTimeout
}

func (c *config) SkipPreflight() bool {
	ch := c.chain.SkipPreflight
	if ch.Valid {
		return ch.Bool
	}
	return c.defaults.SkipPreflight
}

func (c *config) Commitment() rpc.CommitmentType {
	ch := c.chain.Commitment
	if ch.Valid {
		str := ch.String
		var commitment rpc.CommitmentType
		switch str {
		case "processed":
			commitment = rpc.CommitmentProcessed
		case "confirmed":
			commitment = rpc.CommitmentConfirmed
		case "finalized":
			commitment = rpc.CommitmentFinalized
		default:
			c.lggr.Warnf(`Invalid value provided for %s, "%s" - falling back to default "%s"`, "CommitmentType", str, c.defaults.Commitment)
			commitment = rpc.CommitmentConfirmed
		}
		return commitment
	}
	return c.defaults.Commitment
}

func (c *config) FeeEstimatorMode() string {
	ch := c.chain.FeeEstimatorMode
	if ch.Valid {
		return strings.ToLower(ch.String)
	}
	return c.defaults.FeeEstimatorMode
}

func (c *config) ComputeUnitPriceMax() uint64 {
	ch := c.chain.ComputeUnitPriceMax
	if ch.Valid {
		if ch.Int64 >= 0 {
			return uint64(ch.Int64)
		}
		c.lggr.Warnf("Negative value provided for ComputeUnitPriceMax, falling back to default: %d", c.defaults.ComputeUnitPriceMax)
	}
	return c.defaults.ComputeUnitPriceMax
}

func (c *config) ComputeUnitPriceMin() uint64 {
	ch := c.chain.ComputeUnitPriceMin
	if ch.Valid {
		if ch.Int64 >= 0 {
			return uint64(ch.Int64)
		}
		c.lggr.Warnf("Negative value provided for ComputeUnitPriceMin, falling back to default: %d", c.defaults.ComputeUnitPriceMin)
	}
	return c.defaults.ComputeUnitPriceMin
}

func (c *config) ComputeUnitPriceDefault() uint64 {
	ch := c.chain.ComputeUnitPriceDefault
	if ch.Valid {
		if ch.Int64 >= 0 {
			return uint64(ch.Int64)
		}
		c.lggr.Warnf("Negative value provided for ComputeUnitPriceDefault, falling back to default: %d", c.defaults.ComputeUnitPriceDefault)
	}
	return c.defaults.ComputeUnitPriceDefault
}

func (c *config) MaxRetries() *uint {
	ch := c.chain.MaxRetries
	if ch.Valid {
		if ch.Int64 < 0 {
			c.lggr.Warnf(`Negative value provided for %s: %d, falling back to <nil> - let RPC node do a reasonable amount of tries`, "MaxRetries", ch.Int64)
			return nil
		}
		val := uint(ch.Int64)
		return &val
	}
	return c.defaults.MaxRetries
}

func (c *config) FeeBumpPeriod() time.Duration {
	ch := c.chain.FeeBumpPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.FeeBumpPeriod
}

func (c *config) BlockEmissionIdleWarningThreshold() time.Duration {
	return c.defaults.BlockEmissionIdleWarningThreshold
}

func (c *config) FinalityDepth() uint32 {
	return c.defaults.FinalityDepth
}

func (c *config) HeadTrackerHistoryDepth() uint32 {
	return c.defaults.HeadTrackerHistoryDepth
}

func (c *config) HeadTrackerMaxBufferSize() uint32 {
	return c.defaults.HeadTrackerMaxBufferSize
}

func (c *config) HeadTrackerSamplingInterval() time.Duration {
	return c.defaults.HeadTrackerSamplingInterval
}

type Chain struct {
	BalancePollPeriod                 *utils.Duration
	ConfirmPollPeriod                 *utils.Duration
	OCR2CachePollPeriod               *utils.Duration
	OCR2CacheTTL                      *utils.Duration
	TxTimeout                         *utils.Duration
	TxRetryTimeout                    *utils.Duration
	TxConfirmTimeout                  *utils.Duration
	SkipPreflight                     *bool
	Commitment                        *string
	MaxRetries                        *int64
	FeeEstimatorMode                  *string
	ComputeUnitPriceMax               *uint64
	ComputeUnitPriceMin               *uint64
	ComputeUnitPriceDefault           *uint64
	FeeBumpPeriod                     *utils.Duration
	BlockEmissionIdleWarningThreshold *utils.Duration
	FinalityDepth                     *uint32
	HeadTrackerHistoryDepth           *uint32
	HeadTrackerMaxBufferSize          *uint32
	HeadTrackerSamplingInterval       *utils.Duration
}

func (c *Chain) SetDefaults() {
	if c.BalancePollPeriod == nil {
		c.BalancePollPeriod = utils.MustNewDuration(defaultConfigSet.BalancePollPeriod)
	}
	if c.ConfirmPollPeriod == nil {
		c.ConfirmPollPeriod = utils.MustNewDuration(defaultConfigSet.ConfirmPollPeriod)
	}
	if c.OCR2CachePollPeriod == nil {
		c.OCR2CachePollPeriod = utils.MustNewDuration(defaultConfigSet.OCR2CachePollPeriod)
	}
	if c.OCR2CacheTTL == nil {
		c.OCR2CacheTTL = utils.MustNewDuration(defaultConfigSet.OCR2CacheTTL)
	}
	if c.TxTimeout == nil {
		c.TxTimeout = utils.MustNewDuration(defaultConfigSet.TxTimeout)
	}
	if c.TxRetryTimeout == nil {
		c.TxRetryTimeout = utils.MustNewDuration(defaultConfigSet.TxRetryTimeout)
	}
	if c.TxConfirmTimeout == nil {
		c.TxConfirmTimeout = utils.MustNewDuration(defaultConfigSet.TxConfirmTimeout)
	}
	if c.SkipPreflight == nil {
		c.SkipPreflight = &defaultConfigSet.SkipPreflight
	}
	if c.Commitment == nil {
		c.Commitment = (*string)(&defaultConfigSet.Commitment)
	}
	if c.MaxRetries == nil && defaultConfigSet.MaxRetries != nil {
		i := int64(*defaultConfigSet.MaxRetries)
		c.MaxRetries = &i
	}
	if c.FeeEstimatorMode == nil {
		c.FeeEstimatorMode = &defaultConfigSet.FeeEstimatorMode
	}
	if c.ComputeUnitPriceMax == nil {
		c.ComputeUnitPriceMax = &defaultConfigSet.ComputeUnitPriceMax
	}
	if c.ComputeUnitPriceMin == nil {
		c.ComputeUnitPriceMin = &defaultConfigSet.ComputeUnitPriceMin
	}
	if c.ComputeUnitPriceDefault == nil {
		c.ComputeUnitPriceDefault = &defaultConfigSet.ComputeUnitPriceDefault
	}
	if c.FeeBumpPeriod == nil {
		c.FeeBumpPeriod = utils.MustNewDuration(defaultConfigSet.FeeBumpPeriod)
	}
	if c.BlockEmissionIdleWarningThreshold == nil {
		c.BlockEmissionIdleWarningThreshold = utils.MustNewDuration(defaultConfigSet.BlockEmissionIdleWarningThreshold)
	}
	if c.FinalityDepth == nil {
		c.FinalityDepth = &defaultConfigSet.FinalityDepth
	}
	if c.HeadTrackerHistoryDepth == nil {
		c.HeadTrackerHistoryDepth = &defaultConfigSet.HeadTrackerHistoryDepth
	}
	if c.HeadTrackerMaxBufferSize == nil {
		c.HeadTrackerMaxBufferSize = &defaultConfigSet.HeadTrackerMaxBufferSize
	}
	if c.HeadTrackerSamplingInterval == nil {
		c.HeadTrackerSamplingInterval = utils.MustNewDuration(defaultConfigSet.HeadTrackerSamplingInterval)
	}

	return
}

type Node struct {
	Name *string
	URL  *utils.URL
}

func (n *Node) ValidateConfig() (err error) {
	if n.Name == nil {
		err = multierr.Append(err, relaycfg.ErrMissing{Name: "Name", Msg: "required for all nodes"})
	} else if *n.Name == "" {
		err = multierr.Append(err, relaycfg.ErrEmpty{Name: "Name", Msg: "required for all nodes"})
	}
	if n.URL == nil {
		err = multierr.Append(err, relaycfg.ErrMissing{Name: "URL", Msg: "required for all nodes"})
	}
	return
}
