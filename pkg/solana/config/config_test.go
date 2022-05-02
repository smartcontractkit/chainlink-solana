package config

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"
)

// testing configs
var (
	testBalancePoll      = models.MustMakeDuration(1 * time.Minute)
	testConfirmPeriod    = models.MustMakeDuration(2 * time.Minute)
	testCachePeriod      = models.MustMakeDuration(3 * time.Minute)
	testTTL              = models.MustMakeDuration(4 * time.Minute)
	testTxTimeout        = models.MustMakeDuration(5 * time.Minute)
	testTxRetryTimeout   = models.MustMakeDuration(6 * time.Minute)
	testTxConfirmTimeout = models.MustMakeDuration(7 * time.Minute)
	testPreflight        = false
	testCommitment       = "finalized"
)

func TestConfig_ExpectedDefaults(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{}, logger.TestLogger(t))
	configSet := configSet{
		BalancePollPeriod:   cfg.BalancePollPeriod(),
		ConfirmPollPeriod:   cfg.ConfirmPollPeriod(),
		OCR2CachePollPeriod: cfg.OCR2CachePollPeriod(),
		OCR2CacheTTL:        cfg.OCR2CacheTTL(),
		TxTimeout:           cfg.TxTimeout(),
		TxRetryTimeout:      cfg.TxRetryTimeout(),
		TxConfirmTimeout:    cfg.TxConfirmTimeout(),
		SkipPreflight:       cfg.SkipPreflight(),
		Commitment:          cfg.Commitment(),
	}
	assert.Equal(t, defaultConfigSet, configSet)
}

func TestConfig_NewConfig(t *testing.T) {
	dbCfg := db.ChainCfg{
		BalancePollPeriod:   &testBalancePoll,
		ConfirmPollPeriod:   &testConfirmPeriod,
		OCR2CachePollPeriod: &testCachePeriod,
		OCR2CacheTTL:        &testTTL,
		TxTimeout:           &testTxTimeout,
		TxRetryTimeout:      &testTxRetryTimeout,
		TxConfirmTimeout:    &testTxConfirmTimeout,
		SkipPreflight:       null.BoolFrom(testPreflight),
		Commitment:          null.StringFrom(testCommitment),
	}
	cfg := NewConfig(dbCfg, logger.TestLogger(t))
	assert.Equal(t, testBalancePoll.Duration(), cfg.BalancePollPeriod())
	assert.Equal(t, testConfirmPeriod.Duration(), cfg.ConfirmPollPeriod())
	assert.Equal(t, testCachePeriod.Duration(), cfg.OCR2CachePollPeriod())
	assert.Equal(t, testTTL.Duration(), cfg.OCR2CacheTTL())
	assert.Equal(t, testTxTimeout.Duration(), cfg.TxTimeout())
	assert.Equal(t, testTxRetryTimeout.Duration(), cfg.TxRetryTimeout())
	assert.Equal(t, testTxConfirmTimeout.Duration(), cfg.TxConfirmTimeout())
	assert.Equal(t, testPreflight, cfg.SkipPreflight())
	assert.Equal(t, rpc.CommitmentType(testCommitment), cfg.Commitment())
}

func TestConfig_Update(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{}, logger.TestLogger(t))
	dbCfg := db.ChainCfg{
		BalancePollPeriod:   &testBalancePoll,
		ConfirmPollPeriod:   &testConfirmPeriod,
		OCR2CachePollPeriod: &testCachePeriod,
		OCR2CacheTTL:        &testTTL,
		TxTimeout:           &testTxTimeout,
		TxRetryTimeout:      &testTxRetryTimeout,
		TxConfirmTimeout:    &testTxConfirmTimeout,
		SkipPreflight:       null.BoolFrom(testPreflight),
		Commitment:          null.StringFrom(testCommitment),
	}
	cfg.Update(dbCfg)
	assert.Equal(t, testBalancePoll.Duration(), cfg.BalancePollPeriod())
	assert.Equal(t, testConfirmPeriod.Duration(), cfg.ConfirmPollPeriod())
	assert.Equal(t, testCachePeriod.Duration(), cfg.OCR2CachePollPeriod())
	assert.Equal(t, testTTL.Duration(), cfg.OCR2CacheTTL())
	assert.Equal(t, testTxTimeout.Duration(), cfg.TxTimeout())
	assert.Equal(t, testTxRetryTimeout.Duration(), cfg.TxRetryTimeout())
	assert.Equal(t, testTxConfirmTimeout.Duration(), cfg.TxConfirmTimeout())
	assert.Equal(t, testPreflight, cfg.SkipPreflight())
	assert.Equal(t, rpc.CommitmentType(testCommitment), cfg.Commitment())
}

func TestConfig_CommitmentFallback(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{Commitment: null.StringFrom("invalid")}, logger.TestLogger(t))
	assert.Equal(t, rpc.CommitmentConfirmed, cfg.Commitment())
}
