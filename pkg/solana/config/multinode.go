package config

import (
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/config"

	client "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
)

type MultiNode struct {
	// Feature flag
	multiNodeEnabled *bool

	// Node Configs
	pollFailureThreshold       *uint32
	pollInterval               *config.Duration
	selectionMode              *string
	syncThreshold              *uint32
	nodeIsSyncingEnabled       *bool
	leaseDuration              *config.Duration
	finalizedBlockPollInterval *config.Duration
	enforceRepeatableRead      *bool
	deathDeclarationDelay      *config.Duration

	// Chain Configs
	nodeNoNewHeadsThreshold      *config.Duration
	noNewFinalizedHeadsThreshold *config.Duration
	finalityDepth                *uint32
	finalityTagEnabled           *bool
	finalizedBlockOffset         *uint32
}

func (c *MultiNode) MultiNodeEnabled() bool {
	return c.multiNodeEnabled != nil && *c.multiNodeEnabled
}

func (c *MultiNode) PollFailureThreshold() uint32 {
	return *c.pollFailureThreshold
}

func (c *MultiNode) PollInterval() time.Duration {
	return c.pollInterval.Duration()
}

func (c *MultiNode) SelectionMode() string {
	return *c.selectionMode
}

func (c *MultiNode) SyncThreshold() uint32 {
	return *c.syncThreshold
}

func (c *MultiNode) NodeIsSyncingEnabled() bool {
	return *c.nodeIsSyncingEnabled
}

func (c *MultiNode) LeaseDuration() time.Duration { return c.leaseDuration.Duration() }

func (c *MultiNode) FinalizedBlockPollInterval() time.Duration {
	return c.finalizedBlockPollInterval.Duration()
}

func (c *MultiNode) EnforceRepeatableRead() bool { return *c.enforceRepeatableRead }

func (c *MultiNode) DeathDeclarationDelay() time.Duration { return c.deathDeclarationDelay.Duration() }

func (c *MultiNode) NodeNoNewHeadsThreshold() time.Duration {
	return c.nodeNoNewHeadsThreshold.Duration()
}

func (c *MultiNode) NoNewFinalizedHeadsThreshold() time.Duration {
	return c.noNewFinalizedHeadsThreshold.Duration()
}

func (c *MultiNode) FinalityDepth() uint32 { return *c.finalityDepth }

func (c *MultiNode) FinalityTagEnabled() bool { return *c.finalityTagEnabled }

func (c *MultiNode) FinalizedBlockOffset() uint32 { return *c.finalizedBlockOffset }

func (c *MultiNode) SetDefaults() {
	// MultiNode is disabled as it's not fully implemented yet: BCFR-122
	if c.multiNodeEnabled == nil {
		c.multiNodeEnabled = ptr(false)
	}

	/* Node Configs */
	// Failure threshold for polling set to 5 to tolerate some polling failures before taking action.
	if c.pollFailureThreshold == nil {
		c.pollFailureThreshold = ptr(uint32(5))
	}
	// Poll interval is set to 10 seconds to ensure timely updates while minimizing resource usage.
	if c.pollInterval == nil {
		c.pollInterval = config.MustNewDuration(10 * time.Second)
	}
	// Selection mode defaults to priority level to enable using node priorities
	if c.selectionMode == nil {
		c.selectionMode = ptr(client.NodeSelectionModePriorityLevel)
	}
	// The sync threshold is set to 5 to allow for some flexibility in node synchronization before considering it out of sync.
	if c.syncThreshold == nil {
		c.syncThreshold = ptr(uint32(5))
	}
	// Lease duration is set to 1 minute by default to allow node locks for a reasonable amount of time.
	if c.leaseDuration == nil {
		c.leaseDuration = config.MustNewDuration(time.Minute)
	}
	// Node syncing is not relevant for Solana and is disabled by default.
	if c.nodeIsSyncingEnabled == nil {
		c.nodeIsSyncingEnabled = ptr(false)
	}
	// The finalized block polling interval is set to 5 seconds to ensure timely updates while minimizing resource usage.
	if c.finalizedBlockPollInterval == nil {
		c.finalizedBlockPollInterval = config.MustNewDuration(5 * time.Second)
	}
	// Repeatable read guarantee should be enforced by default.
	if c.enforceRepeatableRead == nil {
		c.enforceRepeatableRead = ptr(true)
	}
	// The delay before declaring a node dead is set to 10 seconds to give nodes time to recover from temporary issues.
	if c.deathDeclarationDelay == nil {
		c.deathDeclarationDelay = config.MustNewDuration(10 * time.Second)
	}

	/* Chain Configs */
	// Threshold for no new heads is set to 10 seconds, assuming that heads should update at a reasonable pace.
	if c.nodeNoNewHeadsThreshold == nil {
		c.nodeNoNewHeadsThreshold = config.MustNewDuration(10 * time.Second)
	}
	// Similar to heads, finalized heads should be updated within 10 seconds.
	if c.noNewFinalizedHeadsThreshold == nil {
		c.noNewFinalizedHeadsThreshold = config.MustNewDuration(10 * time.Second)
	}
	// Finality tags are used in Solana and enabled by default.
	if c.finalityTagEnabled == nil {
		c.finalityTagEnabled = ptr(true)
	}
	// Finality depth will not be used since finality tags are enabled.
	if c.finalityDepth == nil {
		c.finalityDepth = ptr(uint32(0))
	}
	// Finalized block offset will not be used since finality tags are enabled.
	if c.finalizedBlockOffset == nil {
		c.finalizedBlockOffset = ptr(uint32(0))
	}
}

func (c *MultiNode) SetFrom(f *MultiNode) {
	if f.multiNodeEnabled != nil {
		c.multiNodeEnabled = f.multiNodeEnabled
	}

	// TODO: Try using reflection here to loop through each one

	// Node Configs
	if f.pollFailureThreshold != nil {
		c.pollFailureThreshold = f.pollFailureThreshold
	}
	if f.pollInterval != nil {
		c.pollInterval = f.pollInterval
	}
	if f.selectionMode != nil {
		c.selectionMode = f.selectionMode
	}
	if f.syncThreshold != nil {
		c.syncThreshold = f.syncThreshold
	}
	if f.nodeIsSyncingEnabled != nil {
		c.nodeIsSyncingEnabled = f.nodeIsSyncingEnabled
	}
	if f.leaseDuration != nil {
		c.leaseDuration = f.leaseDuration
	}
	if f.finalizedBlockPollInterval != nil {
		c.finalizedBlockPollInterval = f.finalizedBlockPollInterval
	}
	if f.enforceRepeatableRead != nil {
		c.enforceRepeatableRead = f.enforceRepeatableRead
	}
	if f.deathDeclarationDelay != nil {
		c.deathDeclarationDelay = f.deathDeclarationDelay
	}

	// Chain Configs
	if f.nodeNoNewHeadsThreshold != nil {
		c.nodeNoNewHeadsThreshold = f.nodeNoNewHeadsThreshold
	}
	if f.noNewFinalizedHeadsThreshold != nil {
		c.noNewFinalizedHeadsThreshold = f.noNewFinalizedHeadsThreshold
	}
	if f.finalityDepth != nil {
		c.finalityDepth = f.finalityDepth
	}
	if f.finalityTagEnabled != nil {
		c.finalityTagEnabled = f.finalityTagEnabled
	}
	if f.finalizedBlockOffset != nil {
		c.finalizedBlockOffset = f.finalizedBlockOffset
	}
}
