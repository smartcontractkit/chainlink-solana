package logpoller

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
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
	return scanJSON("SubkeyPaths", p, src)
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

type Decoder interface {
	CreateType(itemType string, _ bool) (any, error)
	Decode(_ context.Context, raw []byte, into any, itemType string) error
}

type EventIdl struct {
	codec.IdlEvent
	codec.IdlTypeDefSlice
}

func (e *EventIdl) Scan(src interface{}) error {
	return scanJSON("EventIdl", e, src)
}

func (e EventIdl) Value() (driver.Value, error) {
	return json.Marshal(map[string]any{
		"IdlEvent":        e.IdlEvent,
		"IdlTypeDefSlice": e.IdlTypeDefSlice,
	})
}

func (e EventIdl) Equal(o EventIdl) bool {
	return reflect.DeepEqual(e, o)
}

func scanJSON(name string, dest, src interface{}) error {
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

// IndexedValue represents a value which can be written to, read from, or compared to an indexed BYTEA
// postgres field. Maps, structs, and slices or arrays (of anything but byte) are not supported. For signed
// or unsigned integer types, strings, or byte arrays, the SQL operators <, =, & > should work in the expected
// way.
type IndexedValue []byte

func (v *IndexedValue) FromUint64(u uint64) {
	*v = make([]byte, 8)
	binary.BigEndian.PutUint64(*v, u)
}

func (v *IndexedValue) FromInt64(i int64) {
	v.FromUint64(uint64(i + math.MaxInt64 + 1)) //nolint gosec passing i=math.MaxInt64 and i=math.MinInt64 are proven safe in TestIndexedValue
}

func (v *IndexedValue) FromFloat64(f float64) {
	if f > 0 {
		v.FromUint64(math.Float64bits(f) + math.MaxInt64 + 1)
		return
	}
	v.FromUint64(math.MaxInt64 + 1 - math.Float64bits(f))
}

func NewIndexedValue(typedVal any) (iVal IndexedValue, err error) {
	// handle 2 simplest cases first
	switch t := typedVal.(type) {
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	}

	// handle numeric types
	v := reflect.ValueOf(typedVal)
	if v.CanUint() {
		iVal.FromUint64(v.Uint())
		return iVal, nil
	}
	if v.CanInt() {
		iVal.FromInt64(v.Int())
		return iVal, nil
	}
	if v.CanFloat() {
		iVal.FromFloat64(v.Float())
		return iVal, nil
	}

	// any length array is fine as long as the element type is byte
	if t := v.Type(); t.Kind() == reflect.Array {
		if t.Elem().Kind() == reflect.Uint8 {
			return v.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("can't create indexed value from type %T", typedVal)
}
