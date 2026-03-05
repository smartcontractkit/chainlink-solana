package devenv

import (
	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"
)

type ProductInfo struct {
	Name      string `toml:"name"`
	Instances int    `toml:"instances"`
}

// Cfg is the top-level environment config, mirroring devenv.Cfg
// but with Solana-specific infrastructure instead of EVM blockchain.
type Cfg struct {
	Products []*ProductInfo `toml:"products"`
	Solana   *SolanaInput   `toml:"solana"`
	Parrot   *ParrotInput   `toml:"parrot"`
	NodeSets []*ns.Input    `toml:"nodesets" validate:"required"`
}

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
