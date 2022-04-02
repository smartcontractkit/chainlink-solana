package event

import (
	"crypto/sha256"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

var (
	SetConfigDiscriminator       []byte
	SetBillingDiscriminator      []byte
	RoundRequestedDiscriminator  []byte
	NewTransmissionDiscriminator []byte
)

func init() {
	sum := sha256.Sum256([]byte(fmt.Sprintf("event:SetConfig")))
	SetConfigDiscriminator = sum[:8]

	sum = sha256.Sum256([]byte(fmt.Sprintf("event:SetBilling")))
	SetBillingDiscriminator = sum[:8]

	sum = sha256.Sum256([]byte(fmt.Sprintf("event:RoundRequested")))
	RoundRequestedDiscriminator = sum[:8]

	sum = sha256.Sum256([]byte(fmt.Sprintf("event:NewTransmission")))
	NewTransmissionDiscriminator = sum[:8]
}

type SetConfig struct {
	ConfigDigest [32]uint8   `json:"config_digest,omitempty"`
	F            uint8       `json:"f,omitempty"`
	Signers      [][20]uint8 `json:"signers,omitempty"`
}

// UnmarshalBinary makes SetConfig implement encoding.BinaryUnmarshaler
// We manually decode the data because gagliardetto/binary deoes not support slices (eg. for Signers)
func (s *SetConfig) UnmarshalBinary(data []byte) error {
	if (len(data)-32-1)%20 != 0 {
		return fmt.Errorf("incorrect size of SetConfig log. It should be 32byte (ConfigDigest) + 1byte (F) + n*20byte (signers). Instead it's %d", len(data))
	}
	copy(s.ConfigDigest[:], data[:32])
	s.F = data[32]
	numSigners := (len(data) - 32 - 1) / 20
	s.Signers = make([][20]uint8, numSigners)
	for i := 0; i < numSigners; i++ {
		copy(s.Signers[i][:], data[32+1+i*20:32+1+(i+1)*20])
	}
	return nil
}

type SetBilling struct {
	ObservationPaymentGJuels  uint32 `json:"observation_payment_gjuels,omitempty"`
	TransmissionPaymentGJuels uint32 `json:"transmission_payment_gjuels,omitempty"`
}

// UnmarshalBinary makes SetBilling implement encoding.BinaryUnmarshaler
func (s *SetBilling) UnmarshalBinary(data []byte) error {
	return bin.NewBinDecoder(data).Decode(s)
}

type RoundRequested struct {
	ConfigDigest [32]uint8        `json:"config_digest,omitempty"`
	Requester    solana.PublicKey `json:"requester,omitempty"`
	Epoch        uint32           `json:"epoch,omitempty"`
	Round        uint8            `json:"round,omitempty"`
}

// UnmarshalBinary makes RoundRequested implement encoding.BinaryUnmarshaler
func (r *RoundRequested) UnmarshalBinary(data []byte) error {
	return bin.NewBinDecoder(data).Decode(r)
}

type NewTransmission struct {
	RoundID               uint32     `json:"round_id,omitempty"`
	ConfigDigest          [32]uint8  `json:"config_digest,omitempty"`
	Answer                bin.Int128 `json:"answer,omitempty"`
	Transmitter           uint8      `json:"transmitter,omitempty"`
	ObservationsTimestamp uint32     `json:"observations_timestamp,omitempty"`
	ObserverCount         uint8      `json:"observer_count,omitempty"`
	Observers             [19]uint8  `json:"observers,omitempty"`
	JuelsPerLamport       uint64     `json:"juels_per_lamport,omitempty"`
	ReimbursementGJuels   uint64     `json:"reimbursement_gjuels,omitempty"`
}

// UnmarshalBinary makes NewTransmission implement encoding.BinaryUnmarshaler
func (n *NewTransmission) UnmarshalBinary(data []byte) error {
	return bin.NewBinDecoder(data).Decode(n)
}
