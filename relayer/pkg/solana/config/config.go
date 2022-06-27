package config

import (
	"net/url"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-relay/relayer/pkg/utils"

	"github.com/smartcontractkit/chainlink-solana/relayer/pkg/solana/db"
	"github.com/smartcontractkit/chainlink-solana/relayer/pkg/solana/logger"
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
}

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

	// Update sets new chain config values.
	Update(db.ChainCfg)
}

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
}

var _ Config = (*config)(nil)

type config struct {
	defaults configSet
	chain    db.ChainCfg
	chainMu  sync.RWMutex
	lggr     logger.Logger
}

// NewConfig returns a Config with defaults overridden by dbcfg.
func NewConfig(dbcfg db.ChainCfg, lggr logger.Logger) *config {
	return &config{
		defaults: defaultConfigSet,
		chain:    dbcfg,
		lggr:     lggr,
	}
}

func (c *config) Update(dbcfg db.ChainCfg) {
	c.chainMu.Lock()
	c.chain = dbcfg
	c.chainMu.Unlock()
}

func (c *config) BalancePollPeriod() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.BalancePollPeriod
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.BalancePollPeriod
}

func (c *config) ConfirmPollPeriod() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.ConfirmPollPeriod
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.ConfirmPollPeriod
}

func (c *config) OCR2CachePollPeriod() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.OCR2CachePollPeriod
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.OCR2CachePollPeriod
}

func (c *config) OCR2CacheTTL() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.OCR2CacheTTL
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.OCR2CacheTTL
}

func (c *config) TxTimeout() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.TxTimeout
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxTimeout
}

func (c *config) TxRetryTimeout() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.TxRetryTimeout
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxRetryTimeout
}

func (c *config) TxConfirmTimeout() time.Duration {
	c.chainMu.RLock()
	ch := c.chain.TxConfirmTimeout
	c.chainMu.RUnlock()
	if ch != nil {
		return ch.Duration()
	}
	return c.defaults.TxConfirmTimeout
}

func (c *config) SkipPreflight() bool {
	c.chainMu.RLock()
	ch := c.chain.SkipPreflight
	c.chainMu.RUnlock()
	if ch.Valid {
		return ch.Bool
	}
	return c.defaults.SkipPreflight
}

func (c *config) Commitment() rpc.CommitmentType {
	c.chainMu.RLock()
	ch := c.chain.Commitment
	c.chainMu.RUnlock()
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

func (c *config) MaxRetries() *uint {
	c.chainMu.RLock()
	ch := c.chain.MaxRetries
	c.chainMu.RUnlock()
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

type Chain struct {
	BalancePollPeriod   *utils.Duration
	ConfirmPollPeriod   *utils.Duration
	OCR2CachePollPeriod *utils.Duration
	OCR2CacheTTL        *utils.Duration
	TxTimeout           *utils.Duration
	TxRetryTimeout      *utils.Duration
	TxConfirmTimeout    *utils.Duration
	SkipPreflight       *bool
	Commitment          *string
	MaxRetries          *int64
}

func (c *Chain) SetFromDB(cfg *db.ChainCfg) error {
	if cfg == nil {
		return nil
	}

	if cfg.BalancePollPeriod != nil {
		c.BalancePollPeriod = utils.MustNewDuration(cfg.BalancePollPeriod.Duration())
	}
	if cfg.ConfirmPollPeriod != nil {
		c.ConfirmPollPeriod = utils.MustNewDuration(cfg.ConfirmPollPeriod.Duration())
	}
	if cfg.OCR2CachePollPeriod != nil {
		c.OCR2CachePollPeriod = utils.MustNewDuration(cfg.OCR2CachePollPeriod.Duration())
	}
	if cfg.OCR2CacheTTL != nil {
		c.OCR2CacheTTL = utils.MustNewDuration(cfg.OCR2CacheTTL.Duration())
	}
	if cfg.TxTimeout != nil {
		c.TxTimeout = utils.MustNewDuration(cfg.TxTimeout.Duration())
	}
	if cfg.TxRetryTimeout != nil {
		c.TxRetryTimeout = utils.MustNewDuration(cfg.TxRetryTimeout.Duration())
	}
	if cfg.TxConfirmTimeout != nil {
		c.TxConfirmTimeout = utils.MustNewDuration(cfg.TxConfirmTimeout.Duration())
	}
	if cfg.SkipPreflight.Valid {
		c.SkipPreflight = &cfg.SkipPreflight.Bool
	}
	if cfg.Commitment.Valid {
		c.Commitment = &cfg.Commitment.String
	}
	if cfg.MaxRetries.Valid {
		c.MaxRetries = &cfg.MaxRetries.Int64
	}
	return nil
}

type Node struct {
	Name string
	URL  *utils.URL
}

func (n *Node) SetFromDB(db db.Node) error {
	n.Name = db.Name
	if db.SolanaURL != "" {
		u, err := url.Parse(db.SolanaURL)
		if err != nil {
			return err
		}
		n.URL = (*utils.URL)(u)
	}
	return nil
}
