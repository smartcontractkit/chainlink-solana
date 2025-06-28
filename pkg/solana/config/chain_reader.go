package config

import (
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/types/solana"
)

const (
	DefaultLogPollerRetention = 30 * 24 * time.Hour
	DefaultMaxLogsKept        = 0
	DefaultStartingBlock      = 0
	DefaultIncludeReverted    = false
)

var _ = PollingFilter(solana.PollingFilter{})

// PollingFilter is directly convertable from solana.PollingFilter, and extends it with methods.
type PollingFilter struct {
	Retention       *time.Duration `json:"retention,omitempty"`     // maximum amount of time to retain logs
	MaxLogsKept     *int64         `json:"maxLogsKept,omitempty"`   // maximum number of logs to retain ( 0 = unlimited )
	StartingBlock   *int64         `json:"startingBlock,omitempty"` // which block to start looking for logs
	IncludeReverted *bool          `json:"includeReverted"`         // whether to include logs emitted by transactions which failed while executing on chain
}

func (f PollingFilter) GetRetention() time.Duration {
	if f.Retention == nil {
		return DefaultLogPollerRetention
	}

	return *f.Retention
}

func (f PollingFilter) GetMaxLogsKept() int64 {
	if f.MaxLogsKept == nil {
		return DefaultMaxLogsKept
	}

	return *f.MaxLogsKept
}

func (f PollingFilter) GetStartingBlock() int64 {
	if f.StartingBlock == nil {
		return DefaultStartingBlock
	}

	return *f.StartingBlock
}

func (f PollingFilter) GetIncludeReverted() bool {
	if f.IncludeReverted == nil {
		return DefaultIncludeReverted
	}
	return *f.IncludeReverted
}

// Deprecated
type ContractReader = solana.ContractReader

// Deprecated
type ChainContractReader = solana.ChainContractReader

// Deprecated
type EventDefinitions = solana.EventDefinitions

// Deprecated
type MultiReader = solana.MultiReader

// Deprecated
type ReadDefinition = solana.ReadDefinition

// Deprecated
type ReadType = solana.ReadType

// Deprecated
const (
	Account = solana.Account
	Event   = solana.Event
)

// Deprecated
type IndexedField = solana.IndexedField
