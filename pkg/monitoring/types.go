package monitoring

import (
	"github.com/gagliardetto/solana-go"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

type Account struct {
	Slot       uint64
	PubKey     solana.PublicKey
	Lamports   uint64
	Owner      solana.PublicKey
	Data       interface{}
	Executable bool
	RentEpoch  uint64
}

type Transmission struct {
	Header             pkgSolana.TransmissionsHeader `json:"header,omitempty"`
	LatestTransmission pkgSolana.Transmission        `json:'latest_transmission,omitempty"`
}

type Log struct {
	Slot      uint64
	Signature []byte
	Err       interface{} // Either Error or nil (if not error)
	Logs      []string
}
