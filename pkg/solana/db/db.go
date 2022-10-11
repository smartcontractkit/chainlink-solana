package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-relay/pkg/utils"
)

type Chain struct {
	ID        string
	Cfg       ChainCfg
	CreatedAt time.Time
	UpdatedAt time.Time
	Enabled   bool
}

type NewNode struct {
	Name          string `json:"name"`
	SolanaChainID string `json:"solanaChainId" db:"solana_chain_id"`
	SolanaURL     string `json:"solanaURL" db:"solana_url"`
}

type Node struct {
	ID            int32
	Name          string
	SolanaChainID string `json:"solanaChainId" db:"solana_chain_id"`
	SolanaURL     string `json:"solanaURL" db:"solana_url"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ChainCfg struct {
	BalancePollPeriod *utils.Duration

	OCR2CachePollPeriod *utils.Duration
	OCR2CacheTTL        *utils.Duration

	TxTimeout         *utils.Duration
	TxConfirmTimeout  *utils.Duration
	ConfirmPollPeriod *utils.Duration

	SkipPreflight null.Bool // to enable or disable preflight checks
	Commitment    null.String
	MaxRetries    null.Int

	FeeEstimatorMode        null.String
	MaxComputeUnitPrice     null.Int
	MinComputeUnitPrice     null.Int
	DefaultComputeUnitPrice null.Int
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
