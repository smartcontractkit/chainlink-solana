package codec

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

// encoder should be initialized with newEncoder
type encoder struct {
	definitions               map[string]Entry
	lenientCodecFromTypeCodec encodings.LenientCodecFromTypeCodec
}

func newEncoder(definitions map[string]Entry) commontypes.Encoder {
	lenientCodecFromTypeCodec := make(encodings.LenientCodecFromTypeCodec)
	for k, v := range definitions {
		lenientCodecFromTypeCodec[k] = v
	}

	return &encoder{
		lenientCodecFromTypeCodec: lenientCodecFromTypeCodec,
		definitions:               definitions,
	}
}

func (e *encoder) Encode(ctx context.Context, item any, itemType string) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from: %v, while encoding %q", r, itemType)
		}
	}()

	if e.lenientCodecFromTypeCodec == nil {
		return nil, fmt.Errorf("encoder is not properly initialised, underlying lenientCodecFromTypeCodec is nil")
	}

	return e.lenientCodecFromTypeCodec.Encode(ctx, item, itemType)
}

func (e *encoder) GetMaxEncodingSize(_ context.Context, n int, itemType string) (int, error) {
	if e.definitions == nil {
		return 0, fmt.Errorf("encoder is not properly initialised, type definitions are nil")
	}

	entry, ok := e.definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return entry.GetCodecType().Size(n)
}
