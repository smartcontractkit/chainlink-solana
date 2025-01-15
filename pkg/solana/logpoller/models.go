package logpoller

import (
	"encoding/base64"
	"fmt"
	"time"
)

type Filter struct {
	ID            int64 // only for internal usage. Values set externally are ignored.
	Name          string
	Address       PublicKey
	EventName     string
	EventSig      EventSignature
	StartingBlock int64
	EventIdl      EventIdl
	SubkeyPaths   SubkeyPaths
	Retention     time.Duration
	MaxLogsKept   int64
	IsDeleted     bool // only for internal usage. Values set externally are ignored.
	IsBackfilled  bool // only for internal usage. Values set externally are ignored.
}

func (f Filter) MatchSameLogs(other Filter) bool {
	return f.Address == other.Address && f.EventSig == other.EventSig &&
		f.EventIdl.Equal(other.EventIdl) && f.SubkeyPaths.Equal(other.SubkeyPaths)
}

// Discriminator returns a 12 character base64-encoded string
//
// This is the base64 encoding of the [8]byte discriminator returned by utils.Discriminator
func (f Filter) Discriminator() string {
	d := Discriminator("event", f.EventName)
	b64encoded := base64.StdEncoding.EncodeToString(d[:])
	if len(b64encoded) != 12 {
		panic(fmt.Sprintf("Assumption Violation: expected encoding/base64 to return 12 character base64-encoding, got %d characters", len(b64encoded)))
	}
	return b64encoded
}

type Log struct {
	ID             int64
	FilterID       int64
	ChainID        string
	LogIndex       int64
	BlockHash      Hash
	BlockNumber    int64
	BlockTimestamp time.Time
	Address        PublicKey
	EventSig       EventSignature
	SubkeyValues   []IndexedValue
	TxHash         Signature
	Data           []byte
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	SequenceNum    int64
}
