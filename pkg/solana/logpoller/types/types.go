package types

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/lib/pq"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
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

func (k PublicKey) String() string {
	return k.ToSolana().String()
}

func PublicKeysToString(keys []PublicKey) string {
	var buf strings.Builder
	buf.WriteString("[")
	for i, key := range keys {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(key.String())
	}
	buf.WriteString("]")
	return buf.String()
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

func (h Hash) String() string {
	return h.ToSolana().String()
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

func (s Signature) String() string {
	return s.ToSolana().String()
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

type SubKeyPaths [][]string

func (p SubKeyPaths) Value() (driver.Value, error) {
	return json.Marshal([][]string(p))
}

func (p *SubKeyPaths) Scan(src interface{}) error {
	return scanJSON("SubKeyPaths", p, src)
}

func (p SubKeyPaths) Equal(o SubKeyPaths) bool {
	return slices.EqualFunc(p, o, slices.Equal)
}

const EventSignatureLength = 8

type EventSignature [EventSignatureLength]byte

func NewEventSignatureFromName(eventName string) EventSignature {
	return EventSignature(solcommoncodec.NewDiscriminatorHashPrefix(eventName, false))
}

func NewMethodSignatureFromName(methodName string) EventSignature {
	return EventSignature(solcommoncodec.NewMethodDiscriminatorHashPrefix(methodName))
}

// Scan implements Scanner for database/sql.
func (s *EventSignature) Scan(src interface{}) error {
	return scanFixedLengthArray("EventSignature", EventSignatureLength, src, s[:])
}

func (s EventSignature) String() string {
	return string(s[:])
}

// Value implements valuer for database/sql.
func (s EventSignature) Value() (driver.Value, error) {
	return s[:], nil
}

type Decoder interface {
	CreateType(itemType string, _ bool) (any, error)
	Decode(_ context.Context, raw []byte, into any, itemType string) error
}

type EventIdl codecv1.EventIDLTypes

func (e *EventIdl) Scan(src interface{}) error {
	return scanJSON("EventIdl", e, src)
}

func (e EventIdl) Value() (driver.Value, error) {
	return json.Marshal(e)
}

func (e EventIdl) Equal(o EventIdl) bool {
	return reflect.DeepEqual(e, o)
}

func (c *ExtraFilterConfig) Scan(src interface{}) error {
	return scanJSON("ExtraFilterConfig", c, src)
}

func (c ExtraFilterConfig) Value() (driver.Value, error) {
	if c.IsEmpty() {
		return nil, nil
	}
	return json.Marshal(c)
}

func scanJSON(name string, dest, src interface{}) error {
	if src == nil {
		return nil
	}

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
	// Golang signed integers are two's complement encoded, so the value will be stored with two's complement encoding.
	// This also matches the EVM implementation that stores raw topics, EVM ABI encoding also uses two's complement
	v.FromUint64(uint64(i)) // nolint gosec two's complement encoding
}

func (v *IndexedValue) FromFloat64(f float64) {
	if f > 0 {
		v.FromUint64(math.Float64bits(f) + math.MaxInt64 + 1)
		return
	}
	v.FromUint64(math.MaxInt64 + 1 - math.Float64bits(f))
}

type IndexedValues []IndexedValue

func (v *IndexedValues) Scan(src interface{}) error {
	byteArray := pq.ByteaArray{}
	err := byteArray.Scan(src)
	if err != nil {
		return fmt.Errorf("failed to scan IndexedValues: %w", err)
	}

	*v = make([]IndexedValue, 0, len(byteArray))
	for _, b := range byteArray {
		*v = append(*v, b)
	}

	return nil
}

func (v IndexedValues) Value() (driver.Value, error) {
	byteArray := make(pq.ByteaArray, len(v))
	for i, b := range v {
		byteArray[i] = b
	}

	return byteArray.Value()
}

func NewIndexedValue(typedVal any) (iVal IndexedValue, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v while creating indexedValue for %T", r, typedVal)
		}
	}()
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
			if v.CanAddr() {
				return v.Bytes(), nil
			}
			result := make([]byte, v.Len())
			l := v.Len()
			for i := 0; i < l; i++ {
				result[i] = byte(v.Index(i).Uint())
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("can't create indexed value from type %T", typedVal)
}

type ReplayStatus int

const (
	ReplayStatusNoRequest ReplayStatus = iota
	ReplayStatusRequested
	ReplayStatusPending
	ReplayStatusComplete
)

func (rs ReplayStatus) String() string {
	switch rs {
	case ReplayStatusNoRequest:
		return "NoRequest"
	case ReplayStatusRequested:
		return "Requested"
	case ReplayStatusPending:
		return "Pending"
	case ReplayStatusComplete:
		return "Complete"
	default:
		return fmt.Sprintf("invalid status: %d", rs) // Handle unknown cases
	}
}
