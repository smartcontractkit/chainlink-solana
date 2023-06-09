package headtracker

type ChainID int

const (
	Mainnet ChainID = iota
	Testnet
	Devnet
	Localnet
)

// String returns the string representation of the Network value.
func (id ChainID) String() string {
	switch id {
	case Mainnet:
		return "mainnet"
	case Testnet:
		return "testnet"
	case Devnet:
		return "devnet"
	case Localnet:
		return "localnet"
	default:
		return "unknown"
	}
}
