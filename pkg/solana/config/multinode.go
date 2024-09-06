package config

import "time"

type MultiNode struct {
	// Feature flag
	multiNodeEnabled bool

	// Node Configs
	pollFailureThreshold       uint32
	pollInterval               time.Duration
	selectionMode              string
	syncThreshold              uint32
	nodeIsSyncingEnabled       bool
	leaseDuration              time.Duration
	finalizedBlockPollInterval time.Duration
	enforceRepeatableRead      bool
	deathDeclarationDelay      time.Duration

	// Chain Configs
	nodeNoNewHeadsThreshold      time.Duration
	noNewFinalizedHeadsThreshold time.Duration
	finalityDepth                uint32
	finalityTagEnabled           bool
	finalizedBlockOffset         uint32
}

func (c *MultiNode) MultiNodeEnabled() bool {
	return c.multiNodeEnabled
}

func (c *MultiNode) PollFailureThreshold() uint32 {
	return c.pollFailureThreshold
}

func (c *MultiNode) PollInterval() time.Duration {
	return c.pollInterval
}

func (c *MultiNode) SelectionMode() string {
	return c.selectionMode
}

func (c *MultiNode) SyncThreshold() uint32 {
	return c.syncThreshold
}

func (c *MultiNode) NodeIsSyncingEnabled() bool {
	return c.nodeIsSyncingEnabled
}

func (c *MultiNode) FinalizedBlockPollInterval() time.Duration {
	return c.finalizedBlockPollInterval
}

func (c *MultiNode) EnforceRepeatableRead() bool {
	return c.enforceRepeatableRead
}

func (c *MultiNode) DeathDeclarationDelay() time.Duration {
	return c.deathDeclarationDelay
}

func (c *MultiNode) NodeNoNewHeadsThreshold() time.Duration {
	return c.nodeNoNewHeadsThreshold
}

func (c *MultiNode) NoNewFinalizedHeadsThreshold() time.Duration {
	return c.noNewFinalizedHeadsThreshold
}

func (c *MultiNode) FinalityDepth() uint32 {
	return c.finalityDepth
}

func (c *MultiNode) FinalityTagEnabled() bool {
	return c.finalityTagEnabled
}

func (c *MultiNode) FinalizedBlockOffset() uint32 {
	return c.finalizedBlockOffset
}

func (c *MultiNode) SetDefaults() {
	c.multiNodeEnabled = false

	// Node Configs
	c.pollFailureThreshold = 5
	c.pollInterval = 10 * time.Second
	c.selectionMode = "HighestHead"
	c.syncThreshold = 5
	c.leaseDuration = 0
	c.nodeIsSyncingEnabled = false
	c.finalizedBlockPollInterval = 5 * time.Second
	c.enforceRepeatableRead = false
	c.deathDeclarationDelay = 10 * time.Second

	// Chain Configs
	c.nodeNoNewHeadsThreshold = 10 * time.Second      // TODO: Value?
	c.noNewFinalizedHeadsThreshold = 10 * time.Second // TODO: Value?
	c.finalityDepth = 0                               // TODO: Value?
	c.finalityTagEnabled = false                      // TODO: Value?
	c.finalizedBlockOffset = 0                        // TODO: Value?
}
