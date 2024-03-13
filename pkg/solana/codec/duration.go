package codec

import (
	"fmt"
	"reflect"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func NewDuration(builder encodings.Builder) encodings.TypeCodec {
	return &duration{
		intEncoder: builder.Int64(),
	}
}

type duration struct {
	intEncoder encodings.TypeCodec
}

var _ encodings.TypeCodec = &duration{}

func (d *duration) Encode(value any, into []byte) ([]byte, error) {
	bi, ok := value.(time.Duration)
	if !ok {
		return nil, fmt.Errorf("%w: expected time.Duration, got %T", types.ErrInvalidType, value)
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
