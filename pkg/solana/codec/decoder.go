package codec

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

// decoder should be initialized with newDecoder
type decoder struct {
	definitions               map[string]Entry
	lenientCodecFromTypeCodec encodings.LenientCodecFromTypeCodec
}

func newDecoder(definitions map[string]Entry) commontypes.Decoder {
	lenientCodecFromTypeCodec := make(encodings.LenientCodecFromTypeCodec)
	for k, v := range definitions {
		lenientCodecFromTypeCodec[k] = v
	}

	return &decoder{
		definitions:               definitions,
		lenientCodecFromTypeCodec: lenientCodecFromTypeCodec,
	}
}

func (d *decoder) Decode(ctx context.Context, raw []byte, into any, itemType string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from: %v, while decoding %q", r, itemType)
		}
	}()

	if d.lenientCodecFromTypeCodec == nil {
		return fmt.Errorf("decoder is not properly initialised, underlying lenientCodecFromTypeCodec is nil")
	}

	return d.lenientCodecFromTypeCodec.Decode(ctx, raw, into, itemType)
}

func (d *decoder) GetMaxDecodingSize(_ context.Context, n int, itemType string) (int, error) {
	if d.definitions == nil {
		return 0, fmt.Errorf("decoder is not properly initialised, type definitions are nil")
	}

	codecEntry, ok := d.definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return codecEntry.GetCodecType().Size(n)
}
