package headtracker

import (
	"time"

	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
)

// This Config serves as a POC for headtracker.
// It should be replaced with a more robust Config
// such as the one in pkg/solana/Config

// TODO: replace this Config with a more robust Config. Requires research
type Config struct {
	Defaults configSet
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
	BlockEmissionIdleWarningThreshold: 30 * time.Second, // TODO: Check this Config value again
	FinalityDepth:                     50,
	HeadTrackerHistoryDepth:           100,
	HeadTrackerMaxBufferSize:          3,
	HeadTrackerSamplingInterval:       1 * time.Second,
	PollingInterval:                   2 * time.Second,
}

func NewConfig() *Config {
	return &Config{
		Defaults: defaultConfigSet,
	}
}

var _ htrktypes.Config = &Config{}

func (c *Config) BlockEmissionIdleWarningThreshold() time.Duration {
	return c.Defaults.BlockEmissionIdleWarningThreshold
}

func (c *Config) FinalityDepth() uint32 {
	return c.Defaults.FinalityDepth
}

func (c *Config) HeadTrackerHistoryDepth() uint32 {
	return c.Defaults.HeadTrackerHistoryDepth
}

func (c *Config) HeadTrackerMaxBufferSize() uint32 {
	return c.Defaults.HeadTrackerMaxBufferSize
}

func (c *Config) HeadTrackerSamplingInterval() time.Duration {
	return c.Defaults.HeadTrackerSamplingInterval
}

func (c *Config) PollingInterval() time.Duration {
	return c.Defaults.PollingInterval
}

func (c *Config) SetHeadTrackerSamplingInterval(d time.Duration) {
	c.Defaults.HeadTrackerSamplingInterval = d
}
