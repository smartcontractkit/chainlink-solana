package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
)

type Node struct {
	ID            int32
	Name          string
	SolanaChainID string `json:"solanaChainId" db:"solana_chain_id"`
	SolanaURL     string `json:"solanaURL" db:"solana_url"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Deprecated
type ChainCfg struct {
	BalancePollPeriod   *config.Duration
	ConfirmPollPeriod   *config.Duration
	OCR2CachePollPeriod *config.Duration
	OCR2CacheTTL        *config.Duration
	TxTimeout           *config.Duration
	TxRetryTimeout      *config.Duration
	TxConfirmTimeout    *config.Duration
	SkipPreflight       null.Bool // to enable or disable preflight checks
	Commitment          null.String
	MaxRetries          null.Int

	FeeEstimatorMode        null.String
	ComputeUnitPriceMax     null.Int
	ComputeUnitPriceMin     null.Int
	ComputeUnitPriceDefault null.Int
	FeeBumpPeriod           *config.Duration
}

func (c *ChainCfg) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, c)
}

func (c *ChainCfg) Value() (driver.Value, error) {
	return json.Marshal(c)
}
