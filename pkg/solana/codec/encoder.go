package codec

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Encoder struct {
	definitions        map[string]Entry
	codecFromTypeCodec encodings.CodecFromTypeCodec
}

var _ commontypes.Encoder = &Encoder{}

func (e *Encoder) Encode(ctx context.Context, item any, itemType string) (res []byte, err error) {
	if e.codecFromTypeCodec == nil {
		e.codecFromTypeCodec = make(encodings.CodecFromTypeCodec)
		for k, v := range e.definitions {
			e.codecFromTypeCodec[k] = v
		}
	}

	return e.codecFromTypeCodec.Encode(ctx, item, itemType)
}

func (e *Encoder) GetMaxEncodingSize(_ context.Context, n int, itemType string) (int, error) {
	entry, ok := e.definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return entry.GetCodecType().Size(n)
}
