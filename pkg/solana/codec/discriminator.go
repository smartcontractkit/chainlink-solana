package codec

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

const discriminatorLength = 8

func NewDiscriminator(name string) encodings.TypeCodec {
	sum := sha256.Sum256([]byte("account:" + name))
	return &discriminator{hashPrefix: sum[:discriminatorLength]}
}

type discriminator struct {
	hashPrefix []byte
}

func (d discriminator) Encode(value any, into []byte) ([]byte, error) {
	if value == nil {
		return append(into, d.hashPrefix...), nil
	}

	raw, ok := value.(*[]byte)
	if !ok {
		return nil, fmt.Errorf("%w: value must be a byte slice got %T", types.ErrInvalidType, value)
	}

	// inject if not specified
	if raw == nil {
		return append(into, d.hashPrefix...), nil
	}

	// Not sure if we should really be encoding accounts...
	if !bytes.Equal(*raw, d.hashPrefix) {
		return nil, fmt.Errorf("%w: invalid discriminator expected %x got %x", types.ErrInvalidType, d.hashPrefix, raw)
	}

	return append(into, *raw...), nil
}

func (d discriminator) Decode(encoded []byte) (any, []byte, error) {
	raw, remaining, err := encodings.SafeDecode(encoded, discriminatorLength, func(raw []byte) []byte { return raw })
	if err != nil {
		return nil, nil, err
	}

	if !bytes.Equal(raw, d.hashPrefix) {
		return nil, nil, fmt.Errorf("%w: invalid discriminator expected %x got %x", types.ErrInvalidEncoding, d.hashPrefix, raw)
	}

	return &raw, remaining, nil
}

func (d discriminator) GetType() reflect.Type {
	// Pointer type so that nil can inject values and so that the NamedCodec won't wrap with no-nil pointer.
	return reflect.TypeOf(&[]byte{})
}

func (d discriminator) Size(_ int) (int, error) {
	return discriminatorLength, nil
}

func (d discriminator) FixedSize() (int, error) {
	return discriminatorLength, nil
}
