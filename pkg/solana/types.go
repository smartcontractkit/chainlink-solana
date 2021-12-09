package solana

import (
	"math/big"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

const (
	// TransmissionsSize indicates how many transmissions are stored
	TransmissionsSize uint32 = 8096

	// answer (int128, 16 bytes), timestamp (uint32, 4 bytes)
	TransmissionLen uint64 = 16 + 4

	// AccountDiscriminator (8 bytes), RoundID (uint32, 4 bytes), Cursor (uint32, 4 bytes)
	CursorOffset uint64 = 8 + 4
	CursorLen    uint64 = 4

	// Report data (61 bytes)
	MedianLen uint64 = 16
	JuelsLen  uint64 = 8
	ReportLen uint64 = 4 + 1 + 32 + MedianLen + JuelsLen // TODO: explain all
)

// State is the struct representing the contract state
type State struct {
	AccountDiscriminator [8]byte // first 8 bytes of the SHA256 of the account’s Rust ident, https://docs.rs/anchor-lang/0.18.2/anchor_lang/attr.account.html
	Nonce                uint8
	Config               Config
	Oracles              Oracles
	LeftoverPayment      [19]LeftoverPayment
	LeftoverPaymentLen   uint8
	Transmissions        solana.PublicKey
}

// SigningKey represents the report signing key
type SigningKey struct {
	Key [20]byte
}

type OffchainConfig struct {
	Version uint64
	Raw     [4096]byte
	Len     uint64
}

func (oc OffchainConfig) Data() []byte {
	return oc.Raw[:oc.Len]
}

// Config contains the configuration of the contract
type Config struct {
	Version                   uint8
	Owner                     solana.PublicKey
	ProposedOwner             solana.PublicKey
	TokenMint                 solana.PublicKey
	TokenVault                solana.PublicKey
	RequesterAccessController solana.PublicKey
	BillingAccessController   solana.PublicKey
	MinAnswer                 bin.Int128
	MaxAnswer                 bin.Int128
	Decimals                  uint8
	Description               [32]byte
	F                         uint8
	ConfigCount               uint32
	LatestConfigDigest        [32]byte
	LatestConfigBlockNumber   uint64
	LatestAggregatorRoundID   uint32
	LatestTransmitter         solana.PublicKey
	Epoch                     uint32
	Round                     uint8
	Billing                   Billing
	Validator                 solana.PublicKey
	FlaggingThreshold         uint32
	OffchainConfig            OffchainConfig
	PendingOffchainConfig     OffchainConfig
}

// Oracles contains the list of oracles
type Oracles struct {
	Raw [19]Oracle
	Len uint8
}

func (o Oracles) Data() []Oracle {
	return o.Raw[:o.Len]
}

// Oracle contains information about the reporting nodes
type Oracle struct {
	Transmitter   solana.PublicKey
	Signer        SigningKey
	Payee         solana.PublicKey
	ProposedPayee solana.PublicKey
	Payment       uint64
	FromRoundID   uint32
}

// LeftoverPayment contains the remaining payment for each oracle
type LeftoverPayment struct {
	Payee  solana.PublicKey
	Amount uint64
}

// Billing contains the payment information
type Billing struct {
	ObservationPayment uint32
}

// Answer contains the current price answer
type Answer struct {
	Data      *big.Int
	Timestamp uint32
}

// Access controller state
type AccessController struct {
	Owner  solana.PublicKey
	Len    uint8
	Access [32]solana.PublicKey
}

// Validator state
type Validator struct {
	Owner                    solana.PublicKey
	ProposedOwner            solana.PublicKey
	RaisingAccessController  solana.PublicKey
	LoweringAccessController solana.PublicKey

	Flags [128]solana.PublicKey
	Len   uint8
}

// CL Core OCR2 job spec RelayConfig member for Solana
type RelayConfig struct {
	// network data
	NodeEndpointRPC string `json:"nodeEndpointRPC"`
	NodeEndpointWS  string `json:"nodeEndpointWS"`

	// on-chain program + 2x state accounts (state + transmissions) + validator programID
	StateID            string `json:"stateID"`
	TransmissionsID    string `json:"transmissionsID"`
	ValidatorProgramID string `json:"validatorProgramID"`
}
