package commoncodec

import (
	"context"
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type decoder struct {
	definitions          map[string]Entry
	lenientFromTypeCodec map[string]encodings.LenientCodecFromTypeCodec
}

func newDecoder(definitions map[string]Entry) commontypes.Decoder {
	return &decoder{
		definitions:          definitions,
		lenientFromTypeCodec: makeCodecFromDefs(definitions),
	}
}

func (d *decoder) Decode(ctx context.Context, raw []byte, into any, itemType string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from: %v, while decoding %q", r, itemType)
		}
	}()

	_, itemType = commoncodec.ItemTyper(itemType).Next()
	head, tail := commoncodec.ItemTyper(itemType).Next()

	codec, ok := d.lenientFromTypeCodec[head]
	if !ok {
		return fmt.Errorf("%w: codec not available for itemType: %s", commontypes.ErrInvalidType, itemType)
	}

	return codec.Decode(ctx, raw, into, tail)
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

func makeCodecFromDefs(definitions map[string]Entry) map[string]encodings.LenientCodecFromTypeCodec {
	// itemType is constructed as a dot-separated string of values that separates contract
	// names from itemType names within the contract
	lenientFromTypeCodec := make(map[string]encodings.LenientCodecFromTypeCodec)
	for key, value := range definitions {
		_, key = commoncodec.ItemTyper(key).Next()
		head, tail := commoncodec.ItemTyper(key).Next()

		if _, ok := lenientFromTypeCodec[head]; !ok {
			lenientFromTypeCodec[head] = make(encodings.LenientCodecFromTypeCodec)
		}

		lenientFromTypeCodec[head][tail] = value
	}

	return lenientFromTypeCodec
}

func MapstructureDecode(src, dest any) error {
	mDecoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(DecoderHooks...),
		Result:     dest,
		Squash:     true,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", commontypes.ErrInvalidType, err)
	}

	if err = mDecoder.Decode(src); err != nil {
		return fmt.Errorf("%w: %w", commontypes.ErrInvalidType, err)
	}

	return nil
}
