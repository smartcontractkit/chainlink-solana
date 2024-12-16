package solanacodec

import (
	"context"
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Encoder struct {
	Definitions map[string]Entry
}

var _ commontypes.Encoder = &Encoder{}

func (e *Encoder) Encode(_ context.Context, item any, itemType string) (res []byte, err error) {
	info, ok := e.Definitions[itemType]
	if !ok {
		return nil, fmt.Errorf("%w: cannot find definition for %s", commontypes.ErrInvalidType, itemType)
	}

	if item != nil {
		rItem := reflect.ValueOf(item)
		myType := info.GetCodecType().GetType()
		if rItem.Kind() == reflect.Pointer && myType.Kind() != reflect.Pointer {
			rItem = reflect.Indirect(rItem)
		}

		if !rItem.IsZero() && rItem.Type() != myType {
			tmp := reflect.New(myType)
			if err := codec.Convert(rItem, tmp, nil); err != nil {
				return nil, err
			}
			item = tmp.Elem().Interface()
		} else {
			item = rItem.Interface()
		}
	}

	return info.Encode(item, nil)
}

func (e *Encoder) GetMaxEncodingSize(_ context.Context, n int, itemType string) (int, error) {
	entry, ok := e.Definitions[itemType]
	if !ok {
		return 0, fmt.Errorf("%w: nil entry", commontypes.ErrInvalidType)
	}
	return entry.GetCodecType().Size(n)
}
