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

func NewDiscriminator(name string, isAccount bool) *Discriminator {
	return &Discriminator{hashPrefix: NewDiscriminatorHashPrefix(name, isAccount)}
}

func NewDiscriminatorHashPrefix(name string, isAccount bool) []byte {
	var sum [32]byte
	if isAccount {
		sum = sha256.Sum256([]byte("account:" + name))
	} else {
		sum = sha256.Sum256([]byte("event:" + name))
	}

	return sum[:discriminatorLength]
}

type Discriminator struct {
	hashPrefix []byte
}

func (d Discriminator) Encode(value any, into []byte) ([]byte, error) {
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

func (d Discriminator) Decode(encoded []byte) (any, []byte, error) {
	raw, remaining, err := encodings.SafeDecode(encoded, discriminatorLength, func(raw []byte) []byte { return raw })
	if err != nil {
		return nil, nil, err
	}

	if !bytes.Equal(raw, d.hashPrefix) {
		return nil, nil, fmt.Errorf("%w: invalid discriminator expected %x got %x", types.ErrInvalidEncoding, d.hashPrefix, raw)
	}

	return &raw, remaining, nil
}

func (d Discriminator) GetType() reflect.Type {
	// Pointer type so that nil can inject values and so that the NamedCodec won't wrap with no-nil pointer.
	return reflect.TypeOf(&[]byte{})
}

func (d Discriminator) Size(_ int) (int, error) {
	return discriminatorLength, nil
}

func (d Discriminator) FixedSize() (int, error) {
	return discriminatorLength, nil
}

type DiscriminatorExtractor struct {
	b64Index [128]byte
}

// NewDiscriminatorExtractor is optimised to extract discriminators from base64 encoded strings faster than the base64 lib.
func NewDiscriminatorExtractor() DiscriminatorExtractor {
	instance := DiscriminatorExtractor{}
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for i := 0; i < len(base64Chars); i++ {
		instance.b64Index[base64Chars[i]] = byte(i)
	}
	return instance
}

// Extract most optimally (around 40% faster than std) decodes the first 8 bytes of a base64 encoded string, which corresponds to a Solana discriminator.
// Extract expects input of > 12 characters which 8 bytes are extracted from, if the input string is less than 12 characters, this will panic.
// Extract doesn't handle base64 padding because discriminators shouldn't have padding.
// If string contains non-Base64 characters (e.g., !, @, space) map to index 0 (ASCII 'A'), and won't be accurate.
func (e *DiscriminatorExtractor) Extract(data string) []byte {
	var decodeBuffer [9]byte
	d := decodeBuffer[:9]
	s := data[:12]

	// base64 decode
	for i := 0; i < 3; i++ {
		// decode base64 chars into associated byte
		c1 := e.b64Index[s[0]]
		c2 := e.b64Index[s[1]]
		c3 := e.b64Index[s[2]]
		c4 := e.b64Index[s[3]]

		// reconstruct raw bytes
		d[0] = (c1 << 2) | (c2 >> 4)
		d[1] = (c2 << 4) | (c3 >> 2)
		d[2] = (c3 << 6) | c4

		// next 3 bytes and next 4 characters
		d = d[3:]
		s = s[4:]
	}

	return decodeBuffer[:discriminatorLength]
}
