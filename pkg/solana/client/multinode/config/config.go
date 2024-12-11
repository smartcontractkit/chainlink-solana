package config

import (
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
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
