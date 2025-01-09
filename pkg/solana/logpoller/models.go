package logpoller

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/utils"
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
	IsDeleted     bool    // only for internal usage. Values set externally are ignored.
	IsBackfilled  bool    // only for internal usage. Values set externally are ignored.
	decoder       Decoder // only for internal usage.
}

func (f Filter) MatchSameLogs(other Filter) bool {
	return f.Address == other.Address && f.EventSig == other.EventSig &&
		f.EventIdl.Equal(other.EventIdl) && f.SubkeyPaths.Equal(other.SubkeyPaths)
}

func (f Filter) Discriminator() string {
	d := utils.Discriminator("event", f.Name)
	return base64.StdEncoding.EncodeToString(d[:])
}

func (f *Filter) CreateType(subKeyPath string) (any, error) {
	itemType := strings.Join([]string{f.Name, subKeyPath}, ".")
	return f.decoder.CreateType(itemType, false) // TODO: what does bool represent? pass true or false?
}

func (f *Filter) DecodeSubKey(ctx context.Context, raw []byte, subKeyPath []string) (any, error) {
	itemType := strings.Join(slices.Concat([]string{f.Name}, subKeyPath), ".")

	val, err := f.decoder.CreateType(itemType, false)
	if err != nil {
		return nil, err
	}
	err = f.decoder.Decode(ctx, raw, val, itemType)
	return val, err
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
