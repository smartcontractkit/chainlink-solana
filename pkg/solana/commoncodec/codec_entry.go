package commoncodec

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Entry interface {
	Encode(value any, into []byte) ([]byte, error)
	Decode(encoded []byte) (any, []byte, error)
	GetCodecType() encodings.TypeCodec
	GetType() reflect.Type
	Modifier() codec.Modifier
	Size(numItems int) (int, error)
	FixedSize() (int, error)
}

type entry struct {
	// TODO this might not be needed in the end, it was handy to make tests simpler
	genericName       string
	chainSpecificName string
	reflectType       reflect.Type
	typeCodec         encodings.TypeCodec
	mod               codec.Modifier
	// includeDiscriminator during Encode adds a discriminator to the encoded bytes under an assumption that the provided value didn't have a discriminator.
	// During Decode includeDiscriminator removes discriminator from bytes under an assumption that the provided struct doesn't need a discriminator.
	includeDiscriminator bool
	discriminator        Discriminator
}

func NewEntry(
	genericName, chainSpecificName string,
	typeCodec encodings.TypeCodec,
	discriminator *Discriminator,
	mod codec.Modifier,
) Entry {
	e := &entry{
		genericName:       genericName,
		chainSpecificName: chainSpecificName,
		reflectType:       typeCodec.GetType(),
		typeCodec:         typeCodec,
		mod:               ensureModifier(mod),
	}

	if discriminator != nil {
		e.discriminator = *discriminator
		e.includeDiscriminator = true
	}

	return e
}

func (e *entry) Encode(value any, into []byte) ([]byte, error) {
	// Special handling for encoding a nil pointer to an empty struct.
	t := e.reflectType
	if value == nil {
		if t.Kind() == reflect.Pointer {
			elem := t.Elem()
			if elem.Kind() == reflect.Struct && elem.NumField() == 0 {
				return []byte{}, nil
			}
		}
		return nil, fmt.Errorf("%w: cannot encode nil value for genericName: %q, chainSpecificName: %q",
			commontypes.ErrInvalidType, e.genericName, e.chainSpecificName)
	}

	encodedVal, err := e.typeCodec.Encode(value, into)
	if err != nil {
		return nil, err
	}

	if e.includeDiscriminator {
		var byt []byte
		hashPrefix := e.discriminator.HashPrefix()
		encodedDisc, err := e.discriminator.Encode(&hashPrefix, byt)
		if err != nil {
			return nil, err
		}
		return append(encodedDisc, encodedVal...), nil
	}

	return encodedVal, nil
}

func (e *entry) Decode(encoded []byte) (any, []byte, error) {
	if e.includeDiscriminator {
		if len(encoded) < DiscriminatorLength {
			return nil, nil, fmt.Errorf("%w: encoded data too short to contain discriminator for genericName: %q, chainSpecificName: %q",
				commontypes.ErrInvalidType, e.genericName, e.chainSpecificName)
		}

		hashPrefix := e.discriminator.HashPrefix()
		if !bytes.Equal(hashPrefix, encoded[:DiscriminatorLength]) {
			return nil, nil, fmt.Errorf("%w: encoded data has a bad discriminator %v, expected %v, for genericName: %q, chainSpecificName: %q",
				commontypes.ErrInvalidType, encoded[:DiscriminatorLength], hashPrefix, e.genericName, e.chainSpecificName)
		}

		encoded = encoded[DiscriminatorLength:]
	}
	return e.typeCodec.Decode(encoded)
}

func (e *entry) GetCodecType() encodings.TypeCodec {
	return e.typeCodec
}

func (e *entry) GetType() reflect.Type {
	return e.reflectType
}

func (e *entry) Modifier() codec.Modifier {
	return e.mod
}

func (e *entry) Size(numItems int) (int, error) {
	return e.typeCodec.Size(numItems)
}

func (e *entry) FixedSize() (int, error) {
	return e.typeCodec.FixedSize()
}

func EntryAsModifierRemoteCodec(entry Entry, itemType string) (commontypes.RemoteCodec, error) {
	lenientFromTypeCodec := make(encodings.LenientCodecFromTypeCodec)
	lenientFromTypeCodec[itemType] = entry

	return codec.NewModifierCodec(lenientFromTypeCodec, entry.Modifier(), DecoderHooks...)
}

func ensureModifier(mod codec.Modifier) codec.Modifier {
	if mod == nil {
		return codec.MultiModifier{}
	}
	return mod
}
