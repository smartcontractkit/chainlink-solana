package types

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/lib/pq"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

// AnchorCPIMethodName is the method name used by Anchor's emit_cpi! macro.
// The discriminator is the first 8 bytes of SHA256("anchor:event").
const AnchorCPIMethodName = "anchor:event"

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

// AnchorCPIEventDiscriminator returns the 8-byte instruction discriminator used by
// Anchor's emit_cpi! macro. Anchor computes SHA256("anchor:event"), interprets the
// first 8 bytes as a big-endian u64, and writes it on-chain as little-endian.
// This matches Anchor's EVENT_IX_TAG_LE constant.
func AnchorCPIEventDiscriminator() EventSignature {
	sum := sha256.Sum256([]byte(AnchorCPIMethodName))
	var sig EventSignature
	copy(sig[:], sum[:EventSignatureLength])
	slices.Reverse(sig[:])
	return sig
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
type IndexedValue = solcommoncodec.IndexedValue

func NewIndexedValue(typedVal any) (IndexedValue, error) {
	return solcommoncodec.NewIndexedValue(typedVal)
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
