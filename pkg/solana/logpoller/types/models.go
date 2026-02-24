package types

import (
	"time"

	"github.com/gagliardetto/solana-go"
)

type Filter struct {
	ID              int64 // only for internal usage. Values set externally are ignored.
	Name            string
	Address         PublicKey
	EventName       string
	EventSig        EventSignature
	StartingBlock   int64
	EventIdl        EventIdl
	SubkeyPaths     SubKeyPaths
	Retention       time.Duration
	MaxLogsKept     int64
	IsDeleted       bool // only for internal usage. Values set externally are ignored.
	IsBackfilled    bool // only for internal usage. Values set externally are ignored.
	IncludeReverted bool
	// Storing CPI Event Filter Configuration in a separate field.
	// Eventually we will want to deobfuscate the CPI Filter in the db.
	ExtraFilterConfig ExtraFilterConfig
}

// ExtraFilterConfig stores CPI Event Filter Configuration.
type ExtraFilterConfig struct {
	DestProgram     PublicKey      `json:"dest_program,omitempty"`
	MethodSignature EventSignature `json:"method_signature,omitempty"`
}

func (c ExtraFilterConfig) IsEmpty() bool {
	return c.DestProgram == PublicKey{} && c.MethodSignature == EventSignature{}
}

func (c ExtraFilterConfig) Equal(other ExtraFilterConfig) bool {
	return c.DestProgram == other.DestProgram && c.MethodSignature == other.MethodSignature
}

func (f Filter) MatchSameLogs(other Filter) bool {
	return f.Address == other.Address && f.EventSig == other.EventSig && f.EventName == other.EventName &&
		f.EventIdl.Equal(other.EventIdl) && f.SubkeyPaths.Equal(other.SubkeyPaths) &&
		f.ExtraFilterConfig.Equal(other.ExtraFilterConfig)
}

func (f Filter) IsCPIFilter() bool {
	return !f.ExtraFilterConfig.IsEmpty()
}

func (f Filter) GetCPIFilterConfig() ExtraFilterConfig {
	return f.ExtraFilterConfig
}

func (f *Filter) SetCPIFilterConfig(cfg ExtraFilterConfig) {
	f.ExtraFilterConfig = cfg
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
	SubkeyValues   IndexedValues
	TxHash         Signature
	Data           []byte
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	SequenceNum    int64
	Error          *string
}

type BlockData struct {
	SlotNumber          uint64
	BlockHeight         uint64
	BlockHash           solana.Hash
	BlockTime           solana.UnixTimeSeconds
	TransactionHash     solana.Signature
	TransactionIndex    int
	TransactionLogIndex uint
	Error               interface{}
}

type ProgramLog struct {
	BlockData
	Text   string
	Prefix string
}

type ProgramEvent struct {
	Program string
	BlockData
	Data  string
	IsCPI bool
}

type ProgramOutput struct {
	Program      string
	Logs         []ProgramLog
	Events       []ProgramEvent
	ComputeUnits uint
	Truncated    bool
	Failed       bool
	ErrorText    string
}

type Block struct {
	Aborted    bool
	SlotNumber uint64
	BlockHash  *solana.Hash
	Events     []ProgramEvent
}
