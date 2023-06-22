package headtracker

import (
	"time"
)

// This config serves as a POC for headtracker.
// It should be replaced with a more robust config
// such as the one in pkg/solana/config

// TODO: replace this config with a more robust config
type config struct {
	defaults configSet
}

type configSet struct {
	BlockEmissionIdleWarningThreshold time.Duration
	FinalityDepth                     uint32
	HeadTrackerHistoryDepth           uint32
	HeadTrackerMaxBufferSize          uint32
	HeadTrackerSamplingInterval       time.Duration
	PollingInterval                   time.Duration
}

var defaultConfigSet = configSet{
	// headtracker
	BlockEmissionIdleWarningThreshold: 30 * time.Second,
	FinalityDepth:                     50,
	HeadTrackerHistoryDepth:           100,
	HeadTrackerMaxBufferSize:          3,
	HeadTrackerSamplingInterval:       1 * time.Second,
	PollingInterval:                   2 * time.Second,
}

func NewConfig() *config {
	return &config{
		defaults: defaultConfigSet,
	}
}

func (c *config) BlockEmissionIdleWarningThreshold() time.Duration {
	return c.defaults.BlockEmissionIdleWarningThreshold
}

func (c *config) FinalityDepth() uint32 {
	return c.defaults.FinalityDepth
}

func (c *config) HeadTrackerHistoryDepth() uint32 {
	return c.defaults.HeadTrackerHistoryDepth
}

func (c *config) HeadTrackerMaxBufferSize() uint32 {
	return c.defaults.HeadTrackerMaxBufferSize
}

func (c *config) HeadTrackerSamplingInterval() time.Duration {
	return c.defaults.HeadTrackerSamplingInterval
}

func (c *config) PollingInterval() time.Duration {
	return c.defaults.PollingInterval
}
