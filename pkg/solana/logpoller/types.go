package logpoller

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

type PublicKey solana.PublicKey

// Scan implements Scanner for database/sql.
func (k *PublicKey) Scan(src interface{}) error {
	return scanFixedLengthArray("PublicKey", solana.PublicKeyLength, src, k[:])
}

// Value implements valuer for database/sql.
func (k PublicKey) Value() (driver.Value, error) {
	return k[:], nil
}

func (k PublicKey) ToSolana() solana.PublicKey {
	return solana.PublicKey(k)
}

type Hash solana.Hash

// Scan implements Scanner for database/sql.
func (h *Hash) Scan(src interface{}) error {
	return scanFixedLengthArray("Hash", solana.PublicKeyLength, src, h[:])
}

// Value implements valuer for database/sql.
func (h Hash) Value() (driver.Value, error) {
	return h[:], nil
}

func (h Hash) ToSolana() solana.Hash {
	return solana.Hash(h)
}

type Signature solana.Signature

// Scan implements Scanner for database/sql.
func (s *Signature) Scan(src interface{}) error {
	return scanFixedLengthArray("Signature", solana.SignatureLength, src, s[:])
}

// Value implements valuer for database/sql.
func (s Signature) Value() (driver.Value, error) {
	return s[:], nil
}

func (s Signature) ToSolana() solana.Signature {
	return solana.Signature(s)
}

func scanFixedLengthArray(name string, maxLength int, src interface{}, dest []byte) error {
	srcB, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("can't scan %T into %s", src, name)
	}
	if len(srcB) != maxLength {
		return fmt.Errorf("can't scan []byte of len %d into %s, want %d", len(srcB), name, maxLength)
	}
	copy(dest, srcB)
	return nil
}

type SubkeyPaths [][]string

func (p SubkeyPaths) Value() (driver.Value, error) {
	return json.Marshal([][]string(p))
}

func (p *SubkeyPaths) Scan(src interface{}) error {
	return scanJson("SubkeyPaths", p, src)
}

func (p SubkeyPaths) Equal(o SubkeyPaths) bool {
	return slices.EqualFunc(p, o, slices.Equal)
}

const EventSignatureLength = 8

type EventSignature [EventSignatureLength]byte

// Scan implements Scanner for database/sql.
func (s *EventSignature) Scan(src interface{}) error {
	return scanFixedLengthArray("EventSignature", EventSignatureLength, src, s[:])
}

// Value implements valuer for database/sql.
func (s EventSignature) Value() (driver.Value, error) {
	return s[:], nil
}

type EventTypeProvider interface {
	CreateType(eventIdl codec.IdlEvent, typedefSlice codec.IdlTypeDefSlice, subKeyPath []string) (any, error)
}

type EventIdl struct {
	codec.IdlEvent
	codec.IdlTypeDefSlice
}

func (e *EventIdl) Scan(src interface{}) error {
	return scanJson("EventIdl", e, src)
}

func (e EventIdl) Value() (driver.Value, error) {
	return json.Marshal(map[string]any{
		"IdlEvent":        e.IdlEvent,
		"IdlTypeDefSlice": e.IdlTypeDefSlice,
	})
}

func (p EventIdl) Equal(o EventIdl) bool {
	return reflect.DeepEqual(p, o)
}

func scanJson(name string, dest, src interface{}) error {
	var bSrc []byte
	switch src := src.(type) {
	case string:
		bSrc = []byte(src)
	case []byte:
		bSrc = src
	default:
		return fmt.Errorf("can't scan %T into %s", src, name)
	}

	if len(bSrc) == 0 || string(bSrc) == "null" {
		return nil
	}

	err := json.Unmarshal(bSrc, dest)
	if err != nil {
		return fmt.Errorf("failed to scan %v into %s: %w", string(bSrc), name, err)
	}

	return nil
}
