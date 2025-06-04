package api

type ChainID string

const (
	Mainnet ChainID = "mainnet" 
	Testnet ChainID = "testnet"
	Devnet ChainID = "devnet"
	Localnet ChainID = "localnet"
	
)

type SolanaConfig struct {
	ChainID ChainID
}
