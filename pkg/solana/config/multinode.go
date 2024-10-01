package config

import (
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/config"

	client "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
)

type MultiNode struct {
	// Feature flag
	Enabled *bool

	// Node Configs
	PollFailureThreshold       *uint32
	PollInterval               *config.Duration
	SelectionMode              *string
	SyncThreshold              *uint32
	NodeIsSyncingEnabled       *bool
	LeaseDuration              *config.Duration
	FinalizedBlockPollInterval *config.Duration
	EnforceRepeatableRead      *bool
	DeathDeclarationDelay      *config.Duration

	// Chain Configs
	NoNewHeadsThreshold          *config.Duration
	NoNewFinalizedHeadsThreshold *config.Duration
	FinalityDepth                *uint32
	FinalityTagEnabled           *bool
	FinalizedBlockOffset         *uint32
}

func (c *MultiNode) GetEnabled() bool {
	return c.Enabled != nil && *c.Enabled
}

func (c *MultiNode) GetPollFailureThreshold() uint32 {
	return *c.PollFailureThreshold
}

func (c *MultiNode) GetPollInterval() time.Duration {
	return c.PollInterval.Duration()
}

func (c *MultiNode) GetSelectionMode() string {
	return *c.SelectionMode
}

func (c *MultiNode) GetSyncThreshold() uint32 {
	return *c.SyncThreshold
}

func (c *MultiNode) GetNodeIsSyncingEnabled() bool {
	return *c.NodeIsSyncingEnabled
}

func (c *MultiNode) GetLeaseDuration() time.Duration { return c.LeaseDuration.Duration() }

func (c *MultiNode) GetFinalizedBlockPollInterval() time.Duration {
	return c.FinalizedBlockPollInterval.Duration()
}

func (c *MultiNode) GetEnforceRepeatableRead() bool { return *c.EnforceRepeatableRead }

func (c *MultiNode) GetDeathDeclarationDelay() time.Duration {
	return c.DeathDeclarationDelay.Duration()
}

func (c *MultiNode) GetNoNewHeadsThreshold() time.Duration {
	return c.NoNewHeadsThreshold.Duration()
}

func (c *MultiNode) GetNoNewFinalizedHeadsThreshold() time.Duration {
	return c.NoNewFinalizedHeadsThreshold.Duration()
}

func (c *MultiNode) GetFinalityDepth() uint32 { return *c.FinalityDepth }

func (c *MultiNode) GetFinalityTagEnabled() bool { return *c.FinalityTagEnabled }

func (c *MultiNode) GetFinalizedBlockOffset() uint32 { return *c.FinalizedBlockOffset }

func (c *MultiNode) SetDefaults() {
	// MultiNode is disabled as it's not fully implemented yet: BCFR-122
	if c.Enabled == nil {
		c.Enabled = ptr(false)
	}

	/* Node Configs */
	// Failure threshold for polling set to 5 to tolerate some polling failures before taking action.
	if c.PollFailureThreshold == nil {
		c.PollFailureThreshold = ptr(uint32(5))
	}
	// Poll interval is set to 10 seconds to ensure timely updates while minimizing resource usage.
	if c.PollInterval == nil {
		c.PollInterval = config.MustNewDuration(10 * time.Second)
	}
	// Selection mode defaults to priority level to enable using node priorities
	if c.SelectionMode == nil {
		c.SelectionMode = ptr(client.NodeSelectionModePriorityLevel)
	}
	// The sync threshold is set to 5 to allow for some flexibility in node synchronization before considering it out of sync.
	if c.SyncThreshold == nil {
		c.SyncThreshold = ptr(uint32(5))
	}
	// Lease duration is set to 1 minute by default to allow node locks for a reasonable amount of time.
	if c.LeaseDuration == nil {
		c.LeaseDuration = config.MustNewDuration(time.Minute)
	}
	// Node syncing is not relevant for Solana and is disabled by default.
	if c.NodeIsSyncingEnabled == nil {
		c.NodeIsSyncingEnabled = ptr(false)
	}
	// The finalized block polling interval is set to 5 seconds to ensure timely updates while minimizing resource usage.
	if c.FinalizedBlockPollInterval == nil {
		c.FinalizedBlockPollInterval = config.MustNewDuration(5 * time.Second)
	}
	// Repeatable read guarantee should be enforced by default.
	if c.EnforceRepeatableRead == nil {
		c.EnforceRepeatableRead = ptr(true)
	}
	// The delay before declaring a node dead is set to 10 seconds to give nodes time to recover from temporary issues.
	if c.DeathDeclarationDelay == nil {
		c.DeathDeclarationDelay = config.MustNewDuration(10 * time.Second)
	}

	/* Chain Configs */
	// Threshold for no new heads is set to 10 seconds, assuming that heads should update at a reasonable pace.
	if c.NoNewHeadsThreshold == nil {
		c.NoNewHeadsThreshold = config.MustNewDuration(10 * time.Second)
	}
	// Similar to heads, finalized heads should be updated within 10 seconds.
	if c.NoNewFinalizedHeadsThreshold == nil {
		c.NoNewFinalizedHeadsThreshold = config.MustNewDuration(10 * time.Second)
	}
	// Finality tags are used in Solana and enabled by default.
	if c.FinalityTagEnabled == nil {
		c.FinalityTagEnabled = ptr(true)
	}
	// Finality depth will not be used since finality tags are enabled.
	if c.FinalityDepth == nil {
		c.FinalityDepth = ptr(uint32(0))
	}
	// Finalized block offset will not be used since finality tags are enabled.
	if c.FinalizedBlockOffset == nil {
		c.FinalizedBlockOffset = ptr(uint32(0))
	}
}

func (c *MultiNode) SetFrom(f *MultiNode) {
	if f.Enabled != nil {
		c.Enabled = f.Enabled
	}

	// Node Configs
	if f.PollFailureThreshold != nil {
		c.PollFailureThreshold = f.PollFailureThreshold
	}
	if f.PollInterval != nil {
		c.PollInterval = f.PollInterval
	}
	if f.SelectionMode != nil {
		c.SelectionMode = f.SelectionMode
	}
	if f.SyncThreshold != nil {
		c.SyncThreshold = f.SyncThreshold
	}
	if f.NodeIsSyncingEnabled != nil {
		c.NodeIsSyncingEnabled = f.NodeIsSyncingEnabled
	}
	if f.LeaseDuration != nil {
		c.LeaseDuration = f.LeaseDuration
	}
	if f.FinalizedBlockPollInterval != nil {
		c.FinalizedBlockPollInterval = f.FinalizedBlockPollInterval
	}
	if f.EnforceRepeatableRead != nil {
		c.EnforceRepeatableRead = f.EnforceRepeatableRead
	}
	if f.DeathDeclarationDelay != nil {
		c.DeathDeclarationDelay = f.DeathDeclarationDelay
	}

	// Chain Configs
	if f.NoNewHeadsThreshold != nil {
		c.NoNewHeadsThreshold = f.NoNewHeadsThreshold
	}
	if f.NoNewFinalizedHeadsThreshold != nil {
		c.NoNewFinalizedHeadsThreshold = f.NoNewFinalizedHeadsThreshold
	}
	if f.FinalityDepth != nil {
		c.FinalityDepth = f.FinalityDepth
	}
	if f.FinalityTagEnabled != nil {
		c.FinalityTagEnabled = f.FinalityTagEnabled
	}
	if f.FinalizedBlockOffset != nil {
		c.FinalizedBlockOffset = f.FinalizedBlockOffset
	}
}
