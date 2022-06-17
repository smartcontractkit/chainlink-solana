package config

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-solana/relayer/pkg/solana/db"
)

// testing configs
var (
	testBalancePoll            = mustDuration(1 * time.Minute)
	testConfirmPeriod          = mustDuration(2 * time.Minute)
	testCachePeriod            = mustDuration(3 * time.Minute)
	testTTL                    = mustDuration(4 * time.Minute)
	testTxTimeout              = mustDuration(5 * time.Minute)
	testTxRetryTimeout         = mustDuration(6 * time.Minute)
	testTxConfirmTimeout       = mustDuration(7 * time.Minute)
	testPreflight              = false
	testCommitment             = "finalized"
	testMaxRetries       int64 = 123
)

func mustDuration(d time.Duration) utils.Duration {
	ud, err := utils.NewDuration(d)
	if err != nil {
		panic(err)
	}
	return ud
}

func TestConfig_ExpectedDefaults(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{}, logger.Test(t))
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
		MaxRetries:          cfg.MaxRetries(),
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
		MaxRetries:          null.IntFrom(testMaxRetries),
	}
	cfg := NewConfig(dbCfg, logger.Test(t))
	assert.Equal(t, testBalancePoll.Duration(), cfg.BalancePollPeriod())
	assert.Equal(t, testConfirmPeriod.Duration(), cfg.ConfirmPollPeriod())
	assert.Equal(t, testCachePeriod.Duration(), cfg.OCR2CachePollPeriod())
	assert.Equal(t, testTTL.Duration(), cfg.OCR2CacheTTL())
	assert.Equal(t, testTxTimeout.Duration(), cfg.TxTimeout())
	assert.Equal(t, testTxRetryTimeout.Duration(), cfg.TxRetryTimeout())
	assert.Equal(t, testTxConfirmTimeout.Duration(), cfg.TxConfirmTimeout())
	assert.Equal(t, testPreflight, cfg.SkipPreflight())
	assert.Equal(t, rpc.CommitmentType(testCommitment), cfg.Commitment())
	assert.EqualValues(t, testMaxRetries, *cfg.MaxRetries())
}

func TestConfig_Update(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{}, logger.Test(t))
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
		MaxRetries:          null.IntFrom(testMaxRetries),
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
	assert.EqualValues(t, testMaxRetries, *cfg.MaxRetries())
}

func TestConfig_CommitmentFallback(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{Commitment: null.StringFrom("invalid")}, logger.Test(t))
	assert.Equal(t, rpc.CommitmentConfirmed, cfg.Commitment())
}

func TestConfig_MaxRetriesNegativeFallback(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{MaxRetries: null.IntFrom(-100)}, logger.Test(t))
	assert.Nil(t, cfg.MaxRetries())
}
