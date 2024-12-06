package config

type Config struct {
	ChainName        string
	ChainID          string
	RPCUrls          []string
	WSUrls           []string
	ProgramAddresses *ProgramAddresses
	PrivateKey       string
}

type ProgramAddresses struct {
	OCR2             string
	AccessController string
	Store            string
}

func DevnetConfig() *Config {
	return &Config{
		ChainName: "solana",
		ChainID:   "devnet",
		// Will be overridden if set in toml
		RPCUrls: []string{"https://api.devnet.solana.com"},
		WSUrls:  []string{"wss://api.devnet.solana.com/"},
	}
}

func LocalNetConfig() *Config {
	return &Config{
		ChainName: "solana",
		ChainID:   "localnet",
		// Will be overridden if set in toml
		RPCUrls: []string{"http://sol:8899"},
		WSUrls:  []string{"ws://sol:8900"},
		ProgramAddresses: &ProgramAddresses{
			OCR2:             "E3j24rx12SyVsG6quKuZPbQqZPkhAUCh8Uek4XrKYD2x",
			AccessController: "2ckhep7Mvy1dExenBqpcdevhRu7CLuuctMcx7G9mWEvo",
			Store:            "9kRNTZmoZSiTBuXC62dzK9E7gC7huYgcmRRhYv3i4osC",
		},
	}
}
