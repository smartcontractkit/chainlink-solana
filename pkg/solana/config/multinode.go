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
	c.multiNodeEnabled = ptr(false)

	// Node Configs
	c.pollFailureThreshold = ptr(uint32(5))
	c.pollInterval = config.MustNewDuration(10 * time.Second)

	c.selectionMode = ptr(client.NodeSelectionModePriorityLevel)

	c.syncThreshold = ptr(uint32(5))

	// Period at which we verify if active node is still highest block number
	c.leaseDuration = config.MustNewDuration(time.Minute)

	c.nodeIsSyncingEnabled = ptr(false)
	c.finalizedBlockPollInterval = config.MustNewDuration(5 * time.Second)
	c.enforceRepeatableRead = ptr(true)
	c.deathDeclarationDelay = config.MustNewDuration(10 * time.Second)

	// Chain Configs
	c.nodeNoNewHeadsThreshold = config.MustNewDuration(10 * time.Second)
	c.noNewFinalizedHeadsThreshold = config.MustNewDuration(10 * time.Second)
	c.finalityDepth = ptr(uint32(0))
	c.finalityTagEnabled = ptr(true)
	c.finalizedBlockOffset = ptr(uint32(0))
}

func (c *MultiNode) SetFrom(fs *MultiNode) {
	if fs.multiNodeEnabled != nil {
		c.multiNodeEnabled = fs.multiNodeEnabled
	}

	// Node Configs
	if fs.pollFailureThreshold != nil {
		c.pollFailureThreshold = fs.pollFailureThreshold
	}
	if fs.pollInterval != nil {
		c.pollInterval = fs.pollInterval
	}
	if fs.selectionMode != nil {
		c.selectionMode = fs.selectionMode
	}
	if fs.syncThreshold != nil {
		c.syncThreshold = fs.syncThreshold
	}
	if fs.nodeIsSyncingEnabled != nil {
		c.nodeIsSyncingEnabled = fs.nodeIsSyncingEnabled
	}
	if fs.leaseDuration != nil {
		c.leaseDuration = fs.leaseDuration
	}
	if fs.finalizedBlockPollInterval != nil {
		c.finalizedBlockPollInterval = fs.finalizedBlockPollInterval
	}
	if fs.enforceRepeatableRead != nil {
		c.enforceRepeatableRead = fs.enforceRepeatableRead
	}
	if fs.deathDeclarationDelay != nil {
		c.deathDeclarationDelay = fs.deathDeclarationDelay
	}

	// Chain Configs
	if fs.nodeNoNewHeadsThreshold != nil {
		c.nodeNoNewHeadsThreshold = fs.nodeNoNewHeadsThreshold
	}
	if fs.noNewFinalizedHeadsThreshold != nil {
		c.noNewFinalizedHeadsThreshold = fs.noNewFinalizedHeadsThreshold
	}
	if fs.finalityDepth != nil {
		c.finalityDepth = fs.finalityDepth
	}
	if fs.finalityTagEnabled != nil {
		c.finalityTagEnabled = fs.finalityTagEnabled
	}
	if fs.finalizedBlockOffset != nil {
		c.finalizedBlockOffset = fs.finalizedBlockOffset
	}
}

func ptr[T any](v T) *T {
	return &v
}
