package solana

import (
	"encoding/json"
	"errors"
	"math/big"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-relay/pkg/monitoring/pb"
	"google.golang.org/protobuf/proto"
)

const (
	AccountDiscriminatorLen uint64 = 8

	// TransmissionLen = Slot, Timestamp, Padding0, Answer, Padding1, Padding2
	TransmissionLen uint64 = 8 + 4 + 4 + 16 + 8 + 8

	// TransmissionsHeaderLen = Version, State, Owner, ProposedOwner, Writer, Description, Decimals, FlaggingThreshold, LatestRoundID, Granularity, LiveLength, LiveCursor, HistoricalCursor
	TransmissionsHeaderLen     uint64 = 1 + 1 + 32 + 32 + 32 + 32 + 1 + 4 + 4 + 1 + 4 + 4 + 4
	TransmissionsHeaderMaxSize uint64 = 192 // max area allocated to transmissions header

	// ReportLen data (61 bytes)
	MedianLen       uint64 = 16
	JuelsLen        uint64 = 8
	ReportHeaderLen uint64 = 4 + 1 + 32 // timestamp (uint32) + number of observers (uint8) + observer array [32]uint8
	ReportLen       uint64 = ReportHeaderLen + MedianLen + JuelsLen

	// MaxOracles is the maximum number of oracles that can be stored onchain
	MaxOracles = 19
	// MaxOffchainConfigLen is the maximum byte length for the encoded offchainconfig
	MaxOffchainConfigLen = 4096
)

// State is the struct representing the contract state
type State struct {
	AccountDiscriminator [8]byte          `json:"account_discriminator,omitempty"` // first 8 bytes of the SHA256 of the account’s Rust ident, https://docs.rs/anchor-lang/0.18.2/anchor_lang/attr.account.html
	Version              uint8            `json:"version,omitempty"`
	Nonce                uint8            `json:"nonce,omitempty"`
	Padding0             uint16           `json:"-"`
	Padding1             uint32           `json:"-"`
	Transmissions        solana.PublicKey `json:transmissions,omitempty"`
	Config               Config           `json:"config,omitempty"`
	OffchainConfig       OffchainConfig   `json:"offchain_config,omitempty"`
	Oracles              Oracles          `json:"oracles,omitempty"`
}

// SigningKey represents the report signing key
type SigningKey struct {
	Key [20]byte `json:"key,omitempty"`
}

type OffchainConfig struct {
	Version uint64
	Raw     [MaxOffchainConfigLen]byte
	Len     uint64
}

func (oc OffchainConfig) Data() ([]byte, error) {
	if oc.Len > MaxOffchainConfigLen {
		return []byte{}, errors.New("OffchainConfig.Len exceeds MaxOffchainConfigLen")
	}
	return oc.Raw[:oc.Len], nil
}

func (oc OffchainConfig) MarshalJSON() ([]byte, error) {
	data, err := oc.Data()
	if err != nil {
		return nil, err
	}
	offchainConfig := pb.OffchainConfigProto{}
	if err := proto.Unmarshal(data, &offchainConfig); err != nil {
		return nil, err
	}
	return json.Marshal(&offchainConfig)
}

// Config contains the configuration of the contract
type Config struct {
	Owner                     solana.PublicKey `json:"owner,omitempty"`
	ProposedOwner             solana.PublicKey `json:"proposed_owner,omitempty"`
	TokenMint                 solana.PublicKey `json:"token_mint,omitempty"`
	TokenVault                solana.PublicKey `json:"token_vault,omitempty"`
	RequesterAccessController solana.PublicKey `json:"requester_access_controller,omitempty"`
	BillingAccessController   solana.PublicKey `json:"billing_access_controller,omitempty"`
	MinAnswer                 bin.Int128       `json:"min_answer,omitempty"`
	MaxAnswer                 bin.Int128       `json:"max_answer,omitempty"`
	F                         uint8            `json:"f,omitempty"`
	Round                     uint8            `json:"round,omitempty"`
	Padding0                  uint16           `json:"-"`
	Epoch                     uint32           `json:"epoch,omitempty"`
	LatestAggregatorRoundID   uint32           `json:"latest_aggregator_round_id,omitempty"`
	LatestTransmitter         solana.PublicKey `json:"latest_transmitter,omitempty"`
	ConfigCount               uint32           `json:"config_count,omitempty"`
	LatestConfigDigest        [32]byte         `json:"latest_config_digest,omitempty"`
	LatestConfigBlockNumber   uint64           `json:"latest_config_block_number,omitempty"`
	Billing                   Billing          `json:"billing,omitempty"`
}

// Oracles contains the list of oracles
type Oracles struct {
	Raw [MaxOracles]Oracle
	Len uint64
}

func (o Oracles) Data() ([]Oracle, error) {
	if o.Len > MaxOracles {
		return []Oracle{}, errors.New("Oracles.Len exceeds MaxOracles")
	}
	return o.Raw[:o.Len], nil
}

func (o Oracles) MarshalJSON() ([]byte, error) {
	oracles, err := o.Data()
	if err != nil {
		return nil, err
	}
	return json.Marshal(oracles)
}

// Oracle contains information about the reporting nodes
type Oracle struct {
	Transmitter   solana.PublicKey `json:"transmitter,omitempty"`
	Signer        SigningKey       `json:"signer,omitempty"`
	Payee         solana.PublicKey `json:"payee,omitempty"`
	ProposedPayee solana.PublicKey `json:"proposed_payee,omitempty"`
	FromRoundID   uint32           `json:"from_round_id,omitempty"`
	Payment       uint64           `json:"payment,omitempty"`
}

// Billing contains the payment information
type Billing struct {
	ObservationPayment  uint32 `json:"observation_payment,omitempty"`
	TransmissionPayment uint32 `json:"transmission_payment,omitempty"`
}

// Answer contains the current price answer
type Answer struct {
	Data      *big.Int `json:"data,omitempty"`
	Timestamp uint32   `json:"timestamp,omitempty"`
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
	Version           uint8            `json:"version,omitempty"`
	State             uint8            `json:"state,omitempty"`
	Owner             solana.PublicKey `json:"owner,omitempty"`
	ProposedOwner     solana.PublicKey `json:"proposed_owner,omitempty"`
	Writer            solana.PublicKey `json:"writer,omitempty"`
	Description       [32]byte         `json:"description,omitempty"`
	Decimals          uint8            `json:"decimals,omitempty"`
	FlaggingThreshold uint32           `json:"flagging_threshold,omitempty"`
	LatestRoundID     uint32           `json:"latest_round_id,omitempty"`
	Granularity       uint8            `json:"granularity,omitempty"`
	LiveLength        uint32           `json:"live_length,omitempty"`
	LiveCursor        uint32           `json:"live_cursor,omitempty"`
	HistoricalCursor  uint32           `json:"historical_cursor,omitempty"`
}

// Transmission struct for decoding individual tranmissions
type Transmission struct {
	Slot      uint64     `json:"slot,omitempty"`
	Timestamp uint32     `json:"timestamp,omitempty"`
	Padding0  uint32     `json:"-"`
	Answer    bin.Int128 `json:"answer,omitempty"`
	Padding1  uint64     `json:"-"`
	Padding2  uint64     `json:"-"`
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
