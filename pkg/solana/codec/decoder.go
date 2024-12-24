package codec

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Decoder struct {
	definitions          map[string]Entry
	lenientFromTypeCodec encodings.LenientCodecFromTypeCodec
}

var _ commontypes.Decoder = &Decoder{}

func (d *Decoder) Decode(ctx context.Context, raw []byte, into any, itemType string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from: %v, while decoding %q", r, itemType)
		}
	}()

	if d.lenientFromTypeCodec == nil {
		d.lenientFromTypeCodec = make(encodings.LenientCodecFromTypeCodec)
		for k, v := range d.definitions {
			d.lenientFromTypeCodec[k] = v
		}
	}

	return d.lenientFromTypeCodec.Decode(ctx, raw, into, itemType)
}

func (d *Decoder) GetMaxDecodingSize(_ context.Context, n int, itemType string) (int, error) {
	codecEntry, ok := d.definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return codecEntry.GetCodecType().Size(n)
}
