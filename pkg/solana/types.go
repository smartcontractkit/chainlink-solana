package solana

import (
	"math/big"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// TransmissionsSize indicates how many transmissions are stored
const TransmissionsSize uint32 = 8096

// State is the struct representing the contract state
type State struct {
	AccountDiscriminator [8]byte // first 8 bytes of the SHA256 of the account’s Rust ident, https://docs.rs/anchor-lang/0.18.2/anchor_lang/attr.account.html
	Nonce                uint8
	Config               Config
	Oracles              [19]Oracle
	LeftoverPayment      [19]LeftoverPayment
	LeftoverPaymentLen   uint8
	Tranmissions         solana.PublicKey
}

// SigningKey represents the report signing key
type SigningKey struct {
	Key [20]byte
}

// Config contains the configuration of the contract
type Config struct {
	Version                   uint8
	Owner                     solana.PublicKey
	TokenMint                 solana.PublicKey
	TokenVault                solana.PublicKey
	RequesterAccessController solana.PublicKey
	BillingAccessController   solana.PublicKey
	MinAnswer                 bin.Int128
	MaxAnswer                 bin.Int128
	Decimals                  uint8
	Description               [32]byte
	F                         uint8
	N                         uint8
	ConfigCount               uint32
	LatestConfigDigest        [32]byte
	LatestConfigBlockNumber   uint64
	LatestAggregatorRoundID   uint32
	Epoch                     uint32
	Round                     uint8
	Billing                   Billing
	Validator                 solana.PublicKey
	FlaggingThreshold         uint32
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
	Answer    *big.Int
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
