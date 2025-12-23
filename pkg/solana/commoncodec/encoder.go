package commoncodec

import (
	"context"
	"fmt"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

// encoder should be initialized with newEncoder
type encoder struct {
	definitions               map[string]Entry
	lenientCodecFromTypeCodec map[string]encodings.LenientCodecFromTypeCodec
}

func newEncoder(definitions map[string]Entry) commontypes.Encoder {
	return &encoder{
		definitions:               definitions,
		lenientCodecFromTypeCodec: makeCodecFromDefs(definitions),
	}
}

func (e *encoder) Encode(ctx context.Context, item any, itemType string) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from: %v, while encoding %q", r, itemType)
		}
	}()

	_, itemType = commoncodec.ItemTyper(itemType).Next()
	head, tail := commoncodec.ItemTyper(itemType).Next()

	codec, ok := e.lenientCodecFromTypeCodec[head]
	if !ok {
		return nil, fmt.Errorf("%w: codec not available for itemType: %s", commontypes.ErrInvalidType, itemType)
	}

	return codec.Encode(ctx, item, tail)
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
