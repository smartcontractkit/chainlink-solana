package codec

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Encoder struct {
	definitions               map[string]Entry
	lenientCodecFromTypeCodec encodings.LenientCodecFromTypeCodec
}

var _ commontypes.Encoder = &Encoder{}

func (e *Encoder) Encode(ctx context.Context, item any, itemType string) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from: %v, while encoding %q", r, itemType)
		}
	}()

	if e.lenientCodecFromTypeCodec == nil {
		e.lenientCodecFromTypeCodec = make(encodings.LenientCodecFromTypeCodec)
		for k, v := range e.definitions {
			e.lenientCodecFromTypeCodec[k] = v
		}
	}

	return e.lenientCodecFromTypeCodec.Encode(ctx, item, itemType)
}

func (e *Encoder) GetMaxEncodingSize(_ context.Context, n int, itemType string) (int, error) {
	entry, ok := e.definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return entry.GetCodecType().Size(n)
}
