package commoncodec

import (
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
)

func NewOption(codec encodings.TypeCodec) encodings.TypeCodec {
	return &option{
		codec,
	}
}

type option struct {
	codec encodings.TypeCodec
}

var _ encodings.TypeCodec = &option{}

func (d *option) Encode(value any, into []byte) ([]byte, error) {
	// encoding is either 0 for None, or 1, bytes... for Some(val)
	if value == nil {
		return append(into, 0), nil
	}

	into = append(into, 1)
	return d.codec.Encode(value, into)
}

func (d *option) Decode(encoded []byte) (any, []byte, error) {
	prefix := encoded[0]
	bytes := encoded[1:]

	// encoding is either 0 for None, or 1, bytes... for Some(val)
	if prefix == 0 {
		return reflect.Zero(d.codec.GetType()).Interface(), encoded[1:], nil
	}

	if prefix != 1 {
		return nil, encoded, fmt.Errorf("expected either 0 or 1, got %v", prefix)
	}

	return d.codec.Decode(bytes)
}

func (d *option) GetType() reflect.Type {
	return d.codec.GetType()
}

func (d *option) Size(val int) (int, error) {
	return d.codec.Size(val)
}

func (d *option) FixedSize() (int, error) {
	return d.codec.FixedSize()
}
