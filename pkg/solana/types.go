package solana

import (
	"math/big"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

const (
	// Slot, Timestamp, Padding0, Answer, Padding1, Padding2
	TransmissionLen uint64 = 8 + 4 + 4 + 16 + 8 + 8
	// Timestamp(uint64), Answer
	TransmissionLenV1 uint64 = 8 + 16

	// Version, State, Owner, ProposedOwner, Writer, Description, Decimals, FlaggingThreshold, LatestRoundID, Granularity, LiveLength, LiveCursor, HistoricalCursor
	HeaderLen uint64 = 1 + 1 + 32 + 32 + 32 + 32 + 1 + 4 + 4 + 1 + 4 + 4 + 4

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
	OffchainConfig       OffchainConfig
	Oracles              Oracles
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

// Billing contains the payment information
type Billing struct {
	ObservationPayment  uint32
	TransmissionPayment uint32
}

// Answer contains the current price answer
type Answer struct {
	Data      *big.Int
	Timestamp uint32
}

// Access controller state
type AccessController struct {
	Owner         solana.PublicKey
	ProposedOwner solana.PublicKey
	Access        [32]solana.PublicKey
	Len           uint64
}

// TransmissionsHeader struct for decoding transmission state header
type TransmissionsHeader struct {
	Version           uint8
	State             uint8
	Owner             solana.PublicKey
	ProposedOwner     solana.PublicKey
	Writer            solana.PublicKey
	Description       [32]byte
	Decimals          uint8
	FlaggingThreshold uint32
	LatestRoundID     uint32
	Granularity       uint8
	LiveLength        uint32
	LiveCursor        uint32
	HistoricalCursor  uint32
}

// Transmission struct for decoding individual tranmissions
type Transmission struct {
	Slot      uint64
	Timestamp uint32
	Padding0  uint32
	Answer    bin.Int128
	Padding1  uint64
	Padding2  uint64
}

// TransmissionV1 struct for parsing results pre-migration
type TransmissionV1 struct {
	Timestamp uint64
	Answer    bin.Int128
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
	TxTimeout    string `json:"txTimeout"`

	// polling parameters [OPTIONAL]
	PollingInterval   string `json:"pollingInterval"`
	PollingCtxTimeout string `json:"pollingCtxTimeout"`
	StaleTimeout      string `json:"staleTimeout"`
}
