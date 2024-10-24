package config

import (
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/config"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
)

// MultiNodeConfig is a wrapper to provide required functions while keeping configs Public
type MultiNodeConfig struct {
	MultiNode
}

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
	NodeNoNewHeadsThreshold      *config.Duration
	NoNewFinalizedHeadsThreshold *config.Duration
	FinalityDepth                *uint32
	FinalityTagEnabled           *bool
	FinalizedBlockOffset         *uint32
}

func (c *MultiNodeConfig) Enabled() bool {
	return c.MultiNode.Enabled != nil && *c.MultiNode.Enabled
}

func (c *MultiNodeConfig) PollFailureThreshold() uint32 {
	return *c.MultiNode.PollFailureThreshold
}

func (c *MultiNodeConfig) PollInterval() time.Duration {
	return c.MultiNode.PollInterval.Duration()
}

func (c *MultiNodeConfig) SelectionMode() string {
	return *c.MultiNode.SelectionMode
}

func (c *MultiNodeConfig) SyncThreshold() uint32 {
	return *c.MultiNode.SyncThreshold
}

func (c *MultiNodeConfig) NodeIsSyncingEnabled() bool {
	return *c.MultiNode.NodeIsSyncingEnabled
}

func (c *MultiNodeConfig) LeaseDuration() time.Duration { return c.MultiNode.LeaseDuration.Duration() }

func (c *MultiNodeConfig) FinalizedBlockPollInterval() time.Duration {
	return c.MultiNode.FinalizedBlockPollInterval.Duration()
}

func (c *MultiNodeConfig) EnforceRepeatableRead() bool { return *c.MultiNode.EnforceRepeatableRead }

func (c *MultiNodeConfig) DeathDeclarationDelay() time.Duration {
	return c.MultiNode.DeathDeclarationDelay.Duration()
}

func (c *MultiNodeConfig) NodeNoNewHeadsThreshold() time.Duration {
	return c.MultiNode.NodeNoNewHeadsThreshold.Duration()
}

func (c *MultiNodeConfig) NoNewFinalizedHeadsThreshold() time.Duration {
	return c.MultiNode.NoNewFinalizedHeadsThreshold.Duration()
}

func (c *MultiNodeConfig) FinalityDepth() uint32 { return *c.MultiNode.FinalityDepth }

func (c *MultiNodeConfig) FinalityTagEnabled() bool { return *c.MultiNode.FinalityTagEnabled }

func (c *MultiNodeConfig) FinalizedBlockOffset() uint32 { return *c.MultiNode.FinalizedBlockOffset }

func (c *MultiNodeConfig) SetDefaults() {
	// MultiNode is disabled as it's not fully implemented yet: BCFR-122
	if c.MultiNode.Enabled == nil {
		c.MultiNode.Enabled = ptr(false)
	}

	/* Node Configs */
	// Failure threshold for polling set to 5 to tolerate some polling failures before taking action.
	if c.MultiNode.PollFailureThreshold == nil {
		c.MultiNode.PollFailureThreshold = ptr(uint32(8))
	}
	// Poll interval is set to 10 seconds to ensure timely updates while minimizing resource usage.
	if c.MultiNode.PollInterval == nil {
		c.MultiNode.PollInterval = config.MustNewDuration(15 * time.Second)
	}
	// Selection mode defaults to priority level to enable using node priorities
	if c.MultiNode.SelectionMode == nil {
		c.MultiNode.SelectionMode = ptr(mn.NodeSelectionModePriorityLevel)
	}
	// The sync threshold is set to 5 to allow for some flexibility in node synchronization before considering it out of sync.
	if c.MultiNode.SyncThreshold == nil {
		c.MultiNode.SyncThreshold = ptr(uint32(50)) // TODO: Increased to 50 for slow test environment
	}
	// Lease duration is set to 1 minute by default to allow node locks for a reasonable amount of time.
	if c.MultiNode.LeaseDuration == nil {
		c.MultiNode.LeaseDuration = config.MustNewDuration(time.Minute)
	}
	// Node syncing is not relevant for Solana and is disabled by default.
	if c.MultiNode.NodeIsSyncingEnabled == nil {
		c.MultiNode.NodeIsSyncingEnabled = ptr(false)
	}
	// The finalized block polling interval is set to 5 seconds to ensure timely updates while minimizing resource usage.
	if c.MultiNode.FinalizedBlockPollInterval == nil {
		c.MultiNode.FinalizedBlockPollInterval = config.MustNewDuration(15 * time.Second)
	}
	// Repeatable read guarantee should be enforced by default.
	if c.MultiNode.EnforceRepeatableRead == nil {
		c.MultiNode.EnforceRepeatableRead = ptr(true)
	}
	// The delay before declaring a node dead is set to 20 seconds to give nodes time to recover from temporary issues.
	if c.MultiNode.DeathDeclarationDelay == nil {
		c.MultiNode.DeathDeclarationDelay = config.MustNewDuration(45 * time.Second)
	}

	/* Chain Configs */
	// Threshold for no new heads is set to 20 seconds, assuming that heads should update at a reasonable pace.
	if c.MultiNode.NodeNoNewHeadsThreshold == nil {
		c.MultiNode.NodeNoNewHeadsThreshold = config.MustNewDuration(45 * time.Second)
	}
	// Similar to heads, finalized heads should be updated within 20 seconds.
	if c.MultiNode.NoNewFinalizedHeadsThreshold == nil {
		c.MultiNode.NoNewFinalizedHeadsThreshold = config.MustNewDuration(45 * time.Second)
	}
	// Finality tags are used in Solana and enabled by default.
	if c.MultiNode.FinalityTagEnabled == nil {
		c.MultiNode.FinalityTagEnabled = ptr(true)
	}
	// Finality depth will not be used since finality tags are enabled.
	if c.MultiNode.FinalityDepth == nil {
		c.MultiNode.FinalityDepth = ptr(uint32(0))
	}
	// Finalized block offset allows for RPCs to be slightly behind the finalized block.
	if c.MultiNode.FinalizedBlockOffset == nil {
		c.MultiNode.FinalizedBlockOffset = ptr(uint32(50)) // TODO: Set to 50 for slow test environment
	}
}

func (c *MultiNodeConfig) SetFrom(f *MultiNodeConfig) {
	if f.MultiNode.Enabled != nil {
		c.MultiNode.Enabled = f.MultiNode.Enabled
	}

	// Node Configs
	if f.MultiNode.PollFailureThreshold != nil {
		c.MultiNode.PollFailureThreshold = f.MultiNode.PollFailureThreshold
	}
	if f.MultiNode.PollInterval != nil {
		c.MultiNode.PollInterval = f.MultiNode.PollInterval
	}
	if f.MultiNode.SelectionMode != nil {
		c.MultiNode.SelectionMode = f.MultiNode.SelectionMode
	}
	if f.MultiNode.SyncThreshold != nil {
		c.MultiNode.SyncThreshold = f.MultiNode.SyncThreshold
	}
	if f.MultiNode.NodeIsSyncingEnabled != nil {
		c.MultiNode.NodeIsSyncingEnabled = f.MultiNode.NodeIsSyncingEnabled
	}
	if f.MultiNode.LeaseDuration != nil {
		c.MultiNode.LeaseDuration = f.MultiNode.LeaseDuration
	}
	if f.MultiNode.FinalizedBlockPollInterval != nil {
		c.MultiNode.FinalizedBlockPollInterval = f.MultiNode.FinalizedBlockPollInterval
	}
	if f.MultiNode.EnforceRepeatableRead != nil {
		c.MultiNode.EnforceRepeatableRead = f.MultiNode.EnforceRepeatableRead
	}
	if f.MultiNode.DeathDeclarationDelay != nil {
		c.MultiNode.DeathDeclarationDelay = f.MultiNode.DeathDeclarationDelay
	}

	// Chain Configs
	if f.MultiNode.NodeNoNewHeadsThreshold != nil {
		c.MultiNode.NodeNoNewHeadsThreshold = f.MultiNode.NodeNoNewHeadsThreshold
	}
	if f.MultiNode.NoNewFinalizedHeadsThreshold != nil {
		c.MultiNode.NoNewFinalizedHeadsThreshold = f.MultiNode.NoNewFinalizedHeadsThreshold
	}
	if f.MultiNode.FinalityDepth != nil {
		c.MultiNode.FinalityDepth = f.MultiNode.FinalityDepth
	}
	if f.MultiNode.FinalityTagEnabled != nil {
		c.MultiNode.FinalityTagEnabled = f.MultiNode.FinalityTagEnabled
	}
	if f.MultiNode.FinalizedBlockOffset != nil {
		c.MultiNode.FinalizedBlockOffset = f.MultiNode.FinalizedBlockOffset
	}
}
