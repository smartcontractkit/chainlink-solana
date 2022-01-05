package config

import "time"

func applyDefaults(cfg *Config) {
	if cfg.Solana.ReadTimeout == 0 {
		cfg.Solana.ReadTimeout = 2 * time.Second
	}
	if cfg.Solana.PollInterval == 0 {
		cfg.Solana.PollInterval = 5 * time.Second
	}
	if cfg.Feeds.RDDReadTimeout == 0 {
		cfg.Feeds.RDDReadTimeout = 1 * time.Second
	}
	if cfg.Feeds.RDDPollInterval == 0 {
		cfg.Feeds.RDDPollInterval = 10 * time.Second
	}
}
