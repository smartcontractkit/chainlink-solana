package codec

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Decoder struct {
	definitions        map[string]Entry
	codecFromTypeCodec encodings.CodecFromTypeCodec
}

var _ commontypes.Decoder = &Decoder{}

func (d *Decoder) Decode(ctx context.Context, raw []byte, into any, itemType string) (err error) {
	if d.codecFromTypeCodec == nil {
		d.codecFromTypeCodec = make(encodings.CodecFromTypeCodec)
		for k, v := range d.definitions {
			d.codecFromTypeCodec[k] = v
		}
	}

	return d.codecFromTypeCodec.Decode(ctx, raw, into, itemType)
}

func (d *Decoder) GetMaxDecodingSize(_ context.Context, n int, itemType string) (int, error) {
	codecEntry, ok := d.definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return codecEntry.GetCodecType().Size(n)
}
