package config

import (
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
)

// Global terra defaults.
var defaultConfigSet = configSet{
	BalancePollPeriod:   5 * time.Second, // poll period for balance monitoring
	ConfirmPollPeriod:   time.Second,     // polling for tx confirmation
	OCR2CachePollPeriod: time.Second,     // cache polling rate
	OCR2CacheTTL:        time.Minute,     // stale cache deadline
	TxTimeout:           time.Minute,     // transaction timeout
	SkipPreflight:       true,            // to enable or disable preflight checks
	Commitment:          rpc.CommitmentConfirmed,
}

type Config interface {
	BalancePollPeriod() time.Duration
	ConfirmPollPeriod() time.Duration
	OCR2CachePollPeriod() time.Duration
	OCR2CacheTTL() time.Duration
	TxTimeout() time.Duration
	SkipPreflight() bool
	Commitment() rpc.CommitmentType

	// Update sets new chain config values.
	Update(db.ChainCfg)
}

type configSet struct {
	BalancePollPeriod   time.Duration
	ConfirmPollPeriod   time.Duration
	OCR2CachePollPeriod time.Duration
	OCR2CacheTTL        time.Duration
	TxTimeout           time.Duration
	SkipPreflight       bool
	Commitment          rpc.CommitmentType
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
			c.lggr.Warnf(invalidFallbackMsg, "CommitmentType", str, c.defaults.Commitment, nil)
			commitment = rpc.CommitmentConfirmed
		}
		return commitment
	}
	return c.defaults.Commitment
}

const invalidFallbackMsg = `Invalid value provided for %s, "%s" - falling back to default "%s": %v`
