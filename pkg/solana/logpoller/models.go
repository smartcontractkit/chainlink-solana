package logpoller

import (
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/lib/pq"
)

type Filter struct {
	ID            int64
	ChainId       string
	Name          string
	Address       solana.PublicKey
	EventName     string
	EventSig      []byte
	StartingBlock int64
	EventIDL      string
	SubKeyPaths   []string
	Retention     int64
	MaxLogsKept   int64
}

type Log struct {
	ID             int64
	FilterId       int64
	ChainId        string
	LogIndex       int64
	BlockHash      solana.Hash
	BlockNumber    int64
	BLockTimestamp time.Time
	Address        solana.PublicKey
	EventSig       []byte
	SubKeyValues   pq.ByteaArray
	TxHash         solana.Signature
	Data           []byte
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	SequenceNum    int64
}
