package config

import "time"

func applyDefaults(cfg *Config) {
	if cfg.Solana.ReadTimeout == 0 {
		cfg.Solana.ReadTimeout = 2 * time.Second
	}
	if cfg.Solana.PollInterval == 0 {
		cfg.Solana.PollInterval = 5 * time.Second
	}
	if cfg.Feeds.RddReadTimeout == 0 {
		cfg.Feeds.RddReadTimeout = 1 * time.Second
	}
	if cfg.Feeds.RddPollInterval == 0 {
		cfg.Feeds.RddPollInterval = 10 * time.Second
	}
}
