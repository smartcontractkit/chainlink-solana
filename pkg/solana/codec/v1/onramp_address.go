package codecv1

import (
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func NewOnRampAddress(builder encodings.Builder) encodings.TypeCodec {
	return &onRampAddress{
		intEncoder: builder.Uint32(),
	}
}

type onRampAddress struct {
	intEncoder encodings.TypeCodec
}

var _ encodings.TypeCodec = &onRampAddress{}

func (d *onRampAddress) Encode(value any, into []byte) ([]byte, error) {
	bi, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("%w: expected []byte, got %T", types.ErrInvalidType, value)
	}

	length := len(bi)
	if length > 64 {
		return nil, fmt.Errorf("%w: expected []byte to be 64 bytes or less, got %v", types.ErrInvalidType, length)
	}
	// assert 64 bytes or less
	var buf [64]byte
	copy(buf[:], bi)

	// 64 bytes, padded, then len u32
	into = append(into, buf[:]...)
	return d.intEncoder.Encode(uint32(length), into)
}

func (d *onRampAddress) Decode(encoded []byte) (any, []byte, error) {
	buf := encoded[0:64]
	encoded = encoded[64:]

	// decode uint32 len
	l, bytes, err := d.intEncoder.Decode(encoded)
	if err != nil {
		return nil, bytes, err
	}

	length, ok := l.(uint32)
	if !ok {
		return nil, bytes, fmt.Errorf("expected uint32, got %T", l)
	}

	return buf[:length], bytes, nil
}

func (d *onRampAddress) GetType() reflect.Type {
	return reflect.TypeOf([]byte{})
}

func (d *onRampAddress) Size(val int) (int, error) {
	// 64 bytes + uint32
	return 64 + 4, nil
}

func (d *onRampAddress) FixedSize() (int, error) {
	// 64 bytes + uint32
	return 64 + 4, nil
}
