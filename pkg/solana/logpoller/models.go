package logpoller

import (
	"time"

	"github.com/lib/pq"
)

type Filter struct {
	ID            int64
	Name          string
	Address       PublicKey
	EventName     string
	EventSig      []byte
	StartingBlock int64
	EventIDL      string
	SubKeyPaths   SubKeyPaths
	Retention     time.Duration
	MaxLogsKept   int64
}

type Log struct {
	ID             int64
	FilterId       int64
	ChainId        string
	LogIndex       int64
	BlockHash      Hash
	BlockNumber    int64
	BLockTimestamp time.Time
	Address        PublicKey
	EventSig       []byte
	SubKeyValues   pq.ByteaArray
	TxHash         Signature
	Data           []byte
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	SequenceNum    int64
}
