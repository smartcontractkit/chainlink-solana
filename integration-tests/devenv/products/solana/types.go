package solana

type ProgramAddresses struct {
	OCR2             string `toml:"ocr2"`
	AccessController string `toml:"access_controller"`
	Store            string `toml:"store"`
}

type OCR2Solana struct {
	NodeCount        int               `toml:"node_count"`
	NumberOfRounds   int               `toml:"number_of_rounds"`
	GauntletPath     string            `toml:"gauntlet_path"`
	GauntletNetwork  string            `toml:"gauntlet_network"`
	OcrAddress       string            `toml:"ocr_address"`
	FeedAddress      string            `toml:"feed_address"`
	LinkAddress      string            `toml:"link_address"`
	VaultAddress     string            `toml:"vault_address"`
	ProposalAddress  string            `toml:"proposal_address"`
	ProgramAddresses *ProgramAddresses `toml:"program_addresses"`
}
