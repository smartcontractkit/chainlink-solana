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
		return "localnet"
	}
}

func StringToChainID(id string) ChainID {
	switch id {
	case "mainnet":
		return Mainnet
	case "testnet":
		return Testnet
	case "devnet":
		return Devnet
	case "localnet":
		return Localnet
	default:
		return Localnet
	}
}
