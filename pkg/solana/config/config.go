package config

import (
	"strings"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/multierr"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
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
}

var _ Config = (*cfg)(nil)

// Deprecated
type cfg struct {
	defaults configSet
	chain    db.ChainCfg
	lggr     logger.Logger
}

// NewConfig returns a Config with defaults overridden by dbcfg.
// Deprecated
func NewConfig(dbcfg db.ChainCfg, lggr logger.Logger) *cfg {
	return &cfg{
		defaults: defaultConfigSet,
		chain:    dbcfg,
		lggr:     lggr,
	}
}

func (c *cfg) BalancePollPeriod() time.Duration {
	ch := c.chain.BalancePollPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.BalancePollPeriod
}

func (c *cfg) ConfirmPollPeriod() time.Duration {
	ch := c.chain.ConfirmPollPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.ConfirmPollPeriod
}

func (c *cfg) OCR2CachePollPeriod() time.Duration {
	ch := c.chain.OCR2CachePollPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.OCR2CachePollPeriod
}

func (c *cfg) OCR2CacheTTL() time.Duration {
	ch := c.chain.OCR2CacheTTL
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.OCR2CacheTTL
}

func (c *cfg) TxTimeout() time.Duration {
	ch := c.chain.TxTimeout
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxTimeout
}

func (c *cfg) TxRetryTimeout() time.Duration {
	ch := c.chain.TxRetryTimeout
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxRetryTimeout
}

func (c *cfg) TxConfirmTimeout() time.Duration {
	ch := c.chain.TxConfirmTimeout
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxConfirmTimeout
}

func (c *cfg) SkipPreflight() bool {
	ch := c.chain.SkipPreflight
	if ch.Valid {
		return ch.Bool
	}
	return c.defaults.SkipPreflight
}

func (c *cfg) Commitment() rpc.CommitmentType {
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

func (c *cfg) FeeEstimatorMode() string {
	ch := c.chain.FeeEstimatorMode
	if ch.Valid {
		return strings.ToLower(ch.String)
	}
	return c.defaults.FeeEstimatorMode
}

func (c *cfg) ComputeUnitPriceMax() uint64 {
	ch := c.chain.ComputeUnitPriceMax
	if ch.Valid {
		if ch.Int64 >= 0 {
			return uint64(ch.Int64)
		}
		c.lggr.Warnf("Negative value provided for ComputeUnitPriceMax, falling back to default: %d", c.defaults.ComputeUnitPriceMax)
	}
	return c.defaults.ComputeUnitPriceMax
}

func (c *cfg) ComputeUnitPriceMin() uint64 {
	ch := c.chain.ComputeUnitPriceMin
	if ch.Valid {
		if ch.Int64 >= 0 {
			return uint64(ch.Int64)
		}
		c.lggr.Warnf("Negative value provided for ComputeUnitPriceMin, falling back to default: %d", c.defaults.ComputeUnitPriceMin)
	}
	return c.defaults.ComputeUnitPriceMin
}

func (c *cfg) ComputeUnitPriceDefault() uint64 {
	ch := c.chain.ComputeUnitPriceDefault
	if ch.Valid {
		if ch.Int64 >= 0 {
			return uint64(ch.Int64)
		}
		c.lggr.Warnf("Negative value provided for ComputeUnitPriceDefault, falling back to default: %d", c.defaults.ComputeUnitPriceDefault)
	}
	return c.defaults.ComputeUnitPriceDefault
}

func (c *cfg) MaxRetries() *uint {
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

func (c *cfg) FeeBumpPeriod() time.Duration {
	ch := c.chain.FeeBumpPeriod
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.FeeBumpPeriod
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
}

func (c *Chain) SetDefaults() {
	if c.BalancePollPeriod == nil {
		c.BalancePollPeriod = config.MustNewDuration(defaultConfigSet.BalancePollPeriod)
	}
	if c.ConfirmPollPeriod == nil {
		c.ConfirmPollPeriod = config.MustNewDuration(defaultConfigSet.ConfirmPollPeriod)
	}
	if c.OCR2CachePollPeriod == nil {
		c.OCR2CachePollPeriod = config.MustNewDuration(defaultConfigSet.OCR2CachePollPeriod)
	}
	if c.OCR2CacheTTL == nil {
		c.OCR2CacheTTL = config.MustNewDuration(defaultConfigSet.OCR2CacheTTL)
	}
	if c.TxTimeout == nil {
		c.TxTimeout = config.MustNewDuration(defaultConfigSet.TxTimeout)
	}
	if c.TxRetryTimeout == nil {
		c.TxRetryTimeout = config.MustNewDuration(defaultConfigSet.TxRetryTimeout)
	}
	if c.TxConfirmTimeout == nil {
		c.TxConfirmTimeout = config.MustNewDuration(defaultConfigSet.TxConfirmTimeout)
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
		c.FeeBumpPeriod = config.MustNewDuration(defaultConfigSet.FeeBumpPeriod)
	}
	return
}

type Node struct {
	Name *string
	URL  *config.URL
}

func (n *Node) ValidateConfig() (err error) {
	if n.Name == nil {
		err = multierr.Append(err, config.ErrMissing{Name: "Name", Msg: "required for all nodes"})
	} else if *n.Name == "" {
		err = multierr.Append(err, config.ErrEmpty{Name: "Name", Msg: "required for all nodes"})
	}
	if n.URL == nil {
		err = multierr.Append(err, config.ErrMissing{Name: "URL", Msg: "required for all nodes"})
	}
	return
}
