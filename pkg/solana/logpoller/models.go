package logpoller

import (
	"time"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

type Filter struct {
	ID            int64 // only for internal usage. Values set externally are ignored.
	Name          string
	Address       PublicKey
	EventName     string
	EventSig      EventSignature
	StartingBlock int64
	EventIdl      EventIdl
	SubKeyPaths   SubKeyPaths
	Retention     time.Duration
	MaxLogsKept   int64
	IsDeleted     bool // only for internal usage. Values set externally are ignored.
	IsBackfilled  bool // only for internal usage. Values set externally are ignored.
}

func (f Filter) MatchSameLogs(other Filter) bool {
	return f.Address == other.Address && f.EventSig == other.EventSig &&
		f.EventIdl.Equal(other.EventIdl) && f.SubKeyPaths.Equal(other.SubKeyPaths)
}

// DiscriminatorRawBytes returns raw discriminator bytes as a string, this string is not base64 encoded and is always len of discriminator which is 8.
func (f Filter) DiscriminatorRawBytes() string {
	return string(codec.NewDiscriminatorHashPrefix(f.EventName, false))
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
	SubKeyValues   []IndexedValue
	TxHash         Signature
	Data           []byte
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	SequenceNum    int64
}
