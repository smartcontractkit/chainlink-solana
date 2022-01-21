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
	TimestampLen    uint64 = 8
	TransmissionLen uint64 = 16 + TimestampLen

	// AccountDiscriminator (8 bytes), RoundID (uint32, 4 bytes), Cursor (uint32, 4 bytes)
	CursorOffset uint64 = 8 + 1 + 32 + 32 + 32 + 1 + 4 + 4 + 1
	CursorLen    uint64 = 4

	// Report data (61 bytes)
	MedianLen uint64 = 16
	JuelsLen  uint64 = 8
	ReportLen uint64 = 4 + 1 + 32 + MedianLen + JuelsLen // TODO: explain all
)

// State is the struct representing the contract state
type State struct {
	AccountDiscriminator [8]byte // first 8 bytes of the SHA256 of the account’s Rust ident, https://docs.rs/anchor-lang/0.18.2/anchor_lang/attr.account.html
	Version              uint8
	Nonce                uint8
	Padding0             uint16
	Padding1             uint32
	Config               Config
	Oracles              Oracles
	LeftoverPayments     LeftoverPayments
	Transmissions        solana.PublicKey
}

// SigningKey represents the report signing key
type SigningKey struct {
	Key [20]byte
}

type LeftoverPayments struct {
	Raw [19]LeftoverPayment
	Len uint64
}

func (lp LeftoverPayments) Data() []LeftoverPayment {
	return lp.Raw[:lp.Len]
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
	Owner                     solana.PublicKey
	ProposedOwner             solana.PublicKey
	TokenMint                 solana.PublicKey
	TokenVault                solana.PublicKey
	RequesterAccessController solana.PublicKey
	BillingAccessController   solana.PublicKey
	MinAnswer                 bin.Int128
	MaxAnswer                 bin.Int128
	F                         uint8
	Round                     uint8
	Padding0                  uint16
	Epoch                     uint32
	LatestAggregatorRoundID   uint32
	LatestTransmitter         solana.PublicKey
	ConfigCount               uint32
	LatestConfigDigest        [32]byte
	LatestConfigBlockNumber   uint64
	Billing                   Billing
	OffchainConfig            OffchainConfig
	PendingOffchainConfig     OffchainConfig
}

// Oracles contains the list of oracles
type Oracles struct {
	Raw [19]Oracle
	Len uint64
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
	FromRoundID   uint32
	Payment       uint64
}

// LeftoverPayment contains the remaining payment for each oracle
type LeftoverPayment struct {
	Payee  solana.PublicKey
	Amount uint64
}

// Billing contains the payment information
type Billing struct {
	ObservationPayment  uint32
	TransmissionPayment uint32
}

// Answer contains the current price answer
type Answer struct {
	Data      *big.Int
	Timestamp uint64
}

// Access controller state
type AccessController struct {
	Owner         solana.PublicKey
	ProposedOwner solana.PublicKey
	Access        [32]solana.PublicKey
	Len           uint64
}

// CL Core OCR2 job spec RelayConfig member for Solana
type RelayConfig struct {
	// network data
	NodeEndpointHTTP string `json:"nodeEndpointHTTP"`

	// state account passed as the ContractID in main job spec
	// on-chain program + transmissions account + store programID
	OCR2ProgramID   string `json:"ocr2ProgramID"`
	TransmissionsID string `json:"transmissionsID"`
	StoreProgramID  string `json:"storeProgramID"`

	// transaction + state parameters [OPTIONAL]
	UsePreflight bool   `json:"usePreflight"`
	Commitment   string `json:"commitment"`

	// polling parameters [OPTIONAL]
	PollingInterval   string `json:"pollingInterval"`
	PollingCtxTimeout string `json:"pollingCtxTimeout"`
	StaleTimeout      string `json:"staleTimeout"`
}
