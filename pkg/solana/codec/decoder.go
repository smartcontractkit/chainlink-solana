package codec

import (
	"context"
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type decoder struct {
	Definitions map[string]Entry
}

var _ commontypes.Decoder = &decoder{}

func (m *decoder) Decode(_ context.Context, raw []byte, into any, itemType string) (err error) {
	item, ok := m.Definitions[itemType]
	if !ok {
		return fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, itemType)
	}

	val, remaining, err := item.Decode(raw)
	if err != nil {
		return err
	}

	if len(remaining) != 0 {
		return fmt.Errorf("%w: remaining bytes after decoding %s", commontypes.ErrInvalidEncoding, itemType)
	}

	return codec.Convert(reflect.ValueOf(val), reflect.ValueOf(into), nil)
}

func (m *decoder) GetMaxDecodingSize(_ context.Context, n int, itemType string) (int, error) {
	entry, ok := m.Definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return entry.GetCodecType().Size(n)
}
