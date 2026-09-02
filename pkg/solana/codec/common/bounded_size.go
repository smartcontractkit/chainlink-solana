package commoncodec

import (
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

// MaxAccountBytes is the Solana per-account data limit. No legitimate account
// payload or program event payload can exceed it, so it doubles as an upper
// bound on the byte footprint of any single decoded vector.
const MaxAccountBytes = 10 * 1024 * 1024

// minVariableElementBytes is the smallest on-wire footprint of an element whose
// size is not fixed. Variable-size elements (nested vectors, strings) always
// carry at least their own 4 byte length prefix.
const minVariableElementBytes = 4

// maxElementCount returns the largest number of elements of the given type that
// can fit within MaxAccountBytes.
//
// The bound is taken over both the on-wire and the in-memory footprint of an
// element, whichever is larger. They diverge for elements without a fixed size:
// a nested vector occupies only a 4 byte length prefix on the wire but a 24 byte
// slice header in memory, so sizing on the wire footprint alone would allow an
// allocation several times over MaxAccountBytes.
func maxElementCount(element encodings.TypeCodec) int {
	elementBytes := minVariableElementBytes
	if fixed, err := element.FixedSize(); err == nil && fixed > 0 {
		elementBytes = fixed
	}

	if inMemory := int(element.GetType().Size()); inMemory > elementBytes {
		elementBytes = inMemory
	}

	return MaxAccountBytes / elementBytes
}

// CheckElementCount reports whether count elements of the given type fit within
// MaxAccountBytes. Use it where the count is known at construction time, such as
// a fixed array length taken from an IDL; slices bound their wire length through
// NewBoundedSize instead.
func CheckElementCount(count int, element encodings.TypeCodec) error {
	if element == nil {
		return fmt.Errorf("%w: element codec must be non-nil", types.ErrInvalidConfig)
	}

	if limit := maxElementCount(element); count < 0 || count > limit {
		return fmt.Errorf("%w: element count %d exceeds the maximum of %d for solana data", types.ErrInvalidConfig, count, limit)
	}

	return nil
}

// NewBoundedSize wraps the length-prefix codec of a slice so that an element
// count whose implied byte footprint exceeds MaxAccountBytes is rejected while
// decoding the prefix itself.
//
// encodings.NewSlice hands the wire length straight to reflect.MakeSlice and
// validates only that it is non-negative, so a 4 byte prefix would otherwise
// let a few bytes of untrusted data allocate gigabytes before the decode fails
// for lack of input. encodings.slice.Decode returns as soon as the size codec
// errors, which is why the bound belongs here rather than around the slice.
//
// The wire format is unchanged: encoding delegates to the wrapped codec.
func NewBoundedSize(size, element encodings.TypeCodec) (encodings.TypeCodec, error) {
	if size == nil || element == nil {
		return nil, fmt.Errorf("%w: size and element codecs must be non-nil", types.ErrInvalidConfig)
	}

	return &boundedSize{sizeEncoder: size, maxCount: maxElementCount(element)}, nil
}

type boundedSize struct {
	sizeEncoder encodings.TypeCodec
	maxCount    int
}

var _ encodings.TypeCodec = &boundedSize{}

func (b *boundedSize) Encode(value any, into []byte) ([]byte, error) {
	return b.sizeEncoder.Encode(value, into)
}

func (b *boundedSize) Decode(encoded []byte) (any, []byte, error) {
	value, bytes, err := b.sizeEncoder.Decode(encoded)
	if err != nil {
		return nil, nil, err
	}

	count, ok := value.(int)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %T is not an int indicating the size", types.ErrInternal, value)
	}

	if count < 0 || count > b.maxCount {
		return nil, nil, fmt.Errorf("%w: element count %d exceeds the maximum of %d for solana data", types.ErrInvalidEncoding, count, b.maxCount)
	}

	return value, bytes, nil
}

// GetType must report int, otherwise encodings.NewSlice rejects this as a size codec.
func (b *boundedSize) GetType() reflect.Type {
	return b.sizeEncoder.GetType()
}

func (b *boundedSize) Size(val int) (int, error) {
	return b.sizeEncoder.Size(val)
}

// FixedSize must delegate, as encodings.slice.Encode relies on it.
func (b *boundedSize) FixedSize() (int, error) {
	return b.sizeEncoder.FixedSize()
}

// NewBoundedSlice builds a slice codec with a 4 byte Borsh length prefix that is
// bounded by MaxAccountBytes.
func NewBoundedSlice(element encodings.TypeCodec, builder encodings.Builder) (encodings.TypeCodec, error) {
	size, err := builder.Int(4)
	if err != nil {
		return nil, err
	}

	bounded, err := NewBoundedSize(size, element)
	if err != nil {
		return nil, err
	}

	return encodings.NewSlice(element, bounded)
}
