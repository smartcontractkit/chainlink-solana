package solana

type SolanaInput struct {
	Image      string       `toml:"image"`
	ChainID    string       `toml:"chain_id"`
	PublicKey  string       `toml:"public_key"`
	PrivateKey string       `toml:"private_key"`
	Secret     string       `toml:"secret"`
	Out        *SolanaOutput `toml:"out"`
}

type SolanaOutput struct {
	InternalHTTPURL string `toml:"internal_http_url"`
	ExternalHTTPURL string `toml:"external_http_url"`
	ExternalWsURL   string `toml:"external_ws_url"`
	ContainerName   string `toml:"container_name"`
}

type ParrotInput struct {
	Out *ParrotOutput `toml:"out"`
}

type ParrotOutput struct {
	InternalEndpoint string `toml:"internal_endpoint"`
	ExternalEndpoint string `toml:"external_endpoint"`
}

type ProgramAddresses struct {
	OCR2             string `toml:"ocr2"`
	AccessController string `toml:"access_controller"`
	Store            string `toml:"store"`
}

type OCR2Solana struct {
	NodeCount        int               `toml:"node_count"`
	NumberOfRounds   int               `toml:"number_of_rounds"`
	GauntletPath     string            `toml:"gauntlet_path"`
	OcrAddress       string            `toml:"ocr_address"`
	FeedAddress      string            `toml:"feed_address"`
	LinkAddress      string            `toml:"link_address"`
	VaultAddress     string            `toml:"vault_address"`
	ProposalAddress  string            `toml:"proposal_address"`
	ProgramAddresses *ProgramAddresses `toml:"program_addresses"`
}
