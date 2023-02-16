package config

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
)

// testing configs
var (
	testBalancePoll                    = mustDuration(1 * time.Minute)
	testConfirmPeriod                  = mustDuration(2 * time.Minute)
	testCachePeriod                    = mustDuration(3 * time.Minute)
	testTTL                            = mustDuration(4 * time.Minute)
	testTxTimeout                      = mustDuration(5 * time.Minute)
	testTxRetryTimeout                 = mustDuration(6 * time.Minute)
	testTxConfirmTimeout               = mustDuration(7 * time.Minute)
	testPreflight                      = false
	testCommitment                     = "finalized"
	testMaxRetries              int64  = 123
	testFeeEstimatorMode               = "block"
	testComputeUnitPriceMax     uint64 = 100_000
	testComputeUnitPriceMin     uint64 = 1
	testComputeUnitPriceDefault uint64 = 10
	testFeeBumpPeriod                  = mustDuration(8 * time.Minute)
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
		BalancePollPeriod:       cfg.BalancePollPeriod(),
		ConfirmPollPeriod:       cfg.ConfirmPollPeriod(),
		OCR2CachePollPeriod:     cfg.OCR2CachePollPeriod(),
		OCR2CacheTTL:            cfg.OCR2CacheTTL(),
		TxTimeout:               cfg.TxTimeout(),
		TxRetryTimeout:          cfg.TxRetryTimeout(),
		TxConfirmTimeout:        cfg.TxConfirmTimeout(),
		SkipPreflight:           cfg.SkipPreflight(),
		Commitment:              cfg.Commitment(),
		MaxRetries:              cfg.MaxRetries(),
		FeeEstimatorMode:        cfg.FeeEstimatorMode(),
		ComputeUnitPriceMax:     cfg.ComputeUnitPriceMax(),
		ComputeUnitPriceMin:     cfg.ComputeUnitPriceMin(),
		ComputeUnitPriceDefault: cfg.ComputeUnitPriceDefault(),
		FeeBumpPeriod:           cfg.FeeBumpPeriod(),
	}
	assert.Equal(t, defaultConfigSet, configSet)
}

func TestConfig_NewConfig(t *testing.T) {
	dbCfg := db.ChainCfg{
		BalancePollPeriod:       &testBalancePoll,
		ConfirmPollPeriod:       &testConfirmPeriod,
		OCR2CachePollPeriod:     &testCachePeriod,
		OCR2CacheTTL:            &testTTL,
		TxTimeout:               &testTxTimeout,
		TxRetryTimeout:          &testTxRetryTimeout,
		TxConfirmTimeout:        &testTxConfirmTimeout,
		SkipPreflight:           null.BoolFrom(testPreflight),
		Commitment:              null.StringFrom(testCommitment),
		MaxRetries:              null.IntFrom(testMaxRetries),
		FeeEstimatorMode:        null.StringFrom(testFeeEstimatorMode),
		ComputeUnitPriceMax:     null.IntFrom(int64(testComputeUnitPriceMax)),
		ComputeUnitPriceMin:     null.IntFrom(int64(testComputeUnitPriceMin)),
		ComputeUnitPriceDefault: null.IntFrom(int64(testComputeUnitPriceDefault)),
		FeeBumpPeriod:           &testFeeBumpPeriod,
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
	assert.Equal(t, testFeeEstimatorMode, cfg.FeeEstimatorMode())
	assert.Equal(t, testComputeUnitPriceMax, cfg.ComputeUnitPriceMax())
	assert.Equal(t, testComputeUnitPriceMin, cfg.ComputeUnitPriceMin())
	assert.Equal(t, testComputeUnitPriceDefault, cfg.ComputeUnitPriceDefault())
	assert.Equal(t, testFeeBumpPeriod.Duration(), cfg.FeeBumpPeriod())
}

func TestConfig_Update(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{}, logger.Test(t))
	dbCfg := db.ChainCfg{
		BalancePollPeriod:       &testBalancePoll,
		ConfirmPollPeriod:       &testConfirmPeriod,
		OCR2CachePollPeriod:     &testCachePeriod,
		OCR2CacheTTL:            &testTTL,
		TxTimeout:               &testTxTimeout,
		TxRetryTimeout:          &testTxRetryTimeout,
		TxConfirmTimeout:        &testTxConfirmTimeout,
		SkipPreflight:           null.BoolFrom(testPreflight),
		Commitment:              null.StringFrom(testCommitment),
		MaxRetries:              null.IntFrom(testMaxRetries),
		FeeEstimatorMode:        null.StringFrom(testFeeEstimatorMode),
		ComputeUnitPriceMax:     null.IntFrom(int64(testComputeUnitPriceMax)),
		ComputeUnitPriceMin:     null.IntFrom(int64(testComputeUnitPriceMin)),
		ComputeUnitPriceDefault: null.IntFrom(int64(testComputeUnitPriceDefault)),
		FeeBumpPeriod:           &testFeeBumpPeriod,
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
	assert.Equal(t, testFeeEstimatorMode, cfg.FeeEstimatorMode())
	assert.Equal(t, testComputeUnitPriceMax, cfg.ComputeUnitPriceMax())
	assert.Equal(t, testComputeUnitPriceMin, cfg.ComputeUnitPriceMin())
	assert.Equal(t, testComputeUnitPriceDefault, cfg.ComputeUnitPriceDefault())
	assert.Equal(t, testFeeBumpPeriod.Duration(), cfg.FeeBumpPeriod())
}

func TestConfig_CommitmentFallback(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{Commitment: null.StringFrom("invalid")}, logger.Test(t))
	assert.Equal(t, rpc.CommitmentConfirmed, cfg.Commitment())
}

func TestConfig_ComputeBudgetPriceFallback(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{
		ComputeUnitPriceMax:     null.IntFrom(-1),
		ComputeUnitPriceMin:     null.IntFrom(-1),
		ComputeUnitPriceDefault: null.IntFrom(-1),
	}, logger.Test(t))
	assert.Equal(t, defaultConfigSet.ComputeUnitPriceMax, cfg.ComputeUnitPriceMax())
	assert.Equal(t, defaultConfigSet.ComputeUnitPriceMin, cfg.ComputeUnitPriceMin())
	assert.Equal(t, defaultConfigSet.ComputeUnitPriceDefault, cfg.ComputeUnitPriceDefault())
}

func TestConfig_MaxRetriesNegativeFallback(t *testing.T) {
	cfg := NewConfig(db.ChainCfg{MaxRetries: null.IntFrom(-100)}, logger.Test(t))
	assert.Nil(t, cfg.MaxRetries())
}

func TestChain_SetFromDB(t *testing.T) {
	for _, tt := range []struct {
		name  string
		dbCfg *db.ChainCfg
		exp   Chain
	}{
		{"nil", nil, Chain{}},
		{"empty", &db.ChainCfg{}, Chain{}},
		{"full", &db.ChainCfg{
			BalancePollPeriod:       utils.MustNewDuration(5 * time.Second),
			ConfirmPollPeriod:       utils.MustNewDuration(500 * time.Millisecond),
			OCR2CachePollPeriod:     utils.MustNewDuration(time.Second),
			OCR2CacheTTL:            utils.MustNewDuration(time.Minute),
			TxTimeout:               utils.MustNewDuration(time.Minute),
			TxRetryTimeout:          utils.MustNewDuration(10 * time.Second),
			TxConfirmTimeout:        utils.MustNewDuration(30 * time.Second),
			SkipPreflight:           null.BoolFrom(true),
			Commitment:              null.StringFrom("confirmed"),
			MaxRetries:              null.IntFrom(0),
			FeeEstimatorMode:        null.StringFrom("block"),
			ComputeUnitPriceMax:     null.IntFrom(100),
			ComputeUnitPriceMin:     null.IntFrom(10),
			ComputeUnitPriceDefault: null.IntFrom(50),
			FeeBumpPeriod:           utils.MustNewDuration(3 * time.Second),
		}, Chain{
			BalancePollPeriod:       utils.MustNewDuration(5 * time.Second),
			ConfirmPollPeriod:       utils.MustNewDuration(500 * time.Millisecond),
			OCR2CachePollPeriod:     utils.MustNewDuration(time.Second),
			OCR2CacheTTL:            utils.MustNewDuration(time.Minute),
			TxTimeout:               utils.MustNewDuration(time.Minute),
			TxRetryTimeout:          utils.MustNewDuration(10 * time.Second),
			TxConfirmTimeout:        utils.MustNewDuration(30 * time.Second),
			SkipPreflight:           ptr(true),
			Commitment:              ptr("confirmed"),
			MaxRetries:              ptr[int64](0),
			FeeEstimatorMode:        ptr("block"),
			ComputeUnitPriceMax:     ptr[uint64](100),
			ComputeUnitPriceMin:     ptr[uint64](10),
			ComputeUnitPriceDefault: ptr[uint64](50),
			FeeBumpPeriod:           utils.MustNewDuration(3 * time.Second),
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var c Chain
			require.NoError(t, c.SetFromDB(tt.dbCfg))
			assert.Equal(t, tt.exp, c)
		})
	}
}

func TestNode_SetFromDB(t *testing.T) {
	for _, tt := range []struct {
		name   string
		dbNode db.Node
		exp    Node
		expErr bool
	}{
		{"empty", db.Node{}, Node{}, false},
		{"url", db.Node{
			Name:      "test-name",
			SolanaURL: "http://fake.test",
		}, Node{
			Name: ptr("test-name"),
			URL:  utils.MustParseURL("http://fake.test"),
		}, false},
		{"url-missing", db.Node{
			Name: "test-name",
		}, Node{
			Name: ptr("test-name"),
		}, false},
		{"url-invalid", db.Node{
			Name:      "test-name",
			SolanaURL: "asdf;lk.asdf.;lk://asdlkvpoicx;",
		}, Node{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var n Node
			err := n.SetFromDB(tt.dbNode)
			if tt.expErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.exp, n)
			}
		})
	}
}
