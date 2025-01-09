package logpoller

import (
	"time"

	"github.com/lib/pq"
)

type Filter struct {
	ID            int64 // only for internal usage. Values set externally are ignored.
	Name          string
	Address       PublicKey
	EventName     string
	EventSig      EventSignature
	StartingBlock int64
	EventIDL      string
	SubkeyPaths   SubkeyPaths
	Retention     time.Duration
	MaxLogsKept   int64
	IsDeleted     bool // only for internal usage. Values set externally are ignored.
	IsBackfilled  bool // only for internal usage. Values set externally are ignored.
}

func (f Filter) MatchSameLogs(other Filter) bool {
	return f.Address == other.Address && f.EventSig == other.EventSig && f.EventIDL == other.EventIDL && f.SubkeyPaths.Equal(other.SubkeyPaths)
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
	SubkeyValues   pq.ByteaArray
	TxHash         Signature
	Data           []byte
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	SequenceNum    int64
}
