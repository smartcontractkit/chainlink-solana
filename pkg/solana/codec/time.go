package codec

import (
	"fmt"
	"reflect"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func NewUnixTimestamp(builder encodings.Builder) encodings.TypeCodec {
	return &timestamp{
		intEncoder: builder.Int64(),
	}
}

type timestamp struct {
	intEncoder encodings.TypeCodec
}

var _ encodings.TypeCodec = &timestamp{}

func (t *timestamp) Encode(value any, into []byte) ([]byte, error) {
	bi, ok := value.(time.Time)
	if !ok {
		return nil, fmt.Errorf("%w: expected big.Int, got %T", types.ErrInvalidType, value)
	}

	return t.intEncoder.Encode(bi.Unix(), into)
}

func (t *timestamp) Decode(encoded []byte) (any, []byte, error) {
	value, bytes, err := t.intEncoder.Decode(encoded)

	bi, ok := value.(int64)
	if !ok {
		return value, bytes, err
	}

	return time.Unix(bi, 0), bytes, nil
}

func (t *timestamp) GetType() reflect.Type {
	return reflect.TypeOf(time.Time{})
}

func (t *timestamp) Size(val int) (int, error) {
	return t.intEncoder.Size(val)
}

func (t *timestamp) FixedSize() (int, error) {
	return t.intEncoder.FixedSize()
}

func NewDuration(builder encodings.Builder) encodings.TypeCodec {
	return &duration{
		intEncoder: builder.Int64(),
	}
}

type duration struct {
	intEncoder encodings.TypeCodec
}

var _ encodings.TypeCodec = &timestamp{}

func (d *duration) Encode(value any, into []byte) ([]byte, error) {
	bi, ok := value.(time.Duration)
	if !ok {
		return nil, fmt.Errorf("%w: expected big.Int, got %T", types.ErrInvalidType, value)
	}

	return d.intEncoder.Encode(int64(bi), into)
}

func (d *duration) Decode(encoded []byte) (any, []byte, error) {
	value, bytes, err := d.intEncoder.Decode(encoded)

	bi, ok := value.(int64)
	if !ok {
		return value, bytes, err
	}

	return time.Duration(bi), bytes, nil
}

func (d *duration) GetType() reflect.Type {
	return reflect.TypeOf(time.Duration(0))
}

func (d *duration) Size(val int) (int, error) {
	return d.intEncoder.Size(val)
}

func (d *duration) FixedSize() (int, error) {
	return d.intEncoder.FixedSize()
}
