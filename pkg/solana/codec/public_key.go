package codec

import (
	"fmt"
	"reflect"

	"github.com/gagliardetto/solana-go"
)

func NewPublicKey() *PublicKey {
	return &PublicKey{}
}

type PublicKey struct{}

func (p PublicKey) Encode(value any, into []byte) ([]byte, error) {
	if value == nil {
		return []byte{}, nil
	}

	switch v := value.(type) {
	case string:
		key, err := solana.PublicKeyFromBase58(v)
		return append(into, key.Bytes()...), err
	case []byte:
		out := solana.PublicKeyFromBytes(v)
		return out.Bytes(), nil
	case *[]byte:
		if v == nil {
			return nil, fmt.Errorf("nil bytes provided for encoding public key")
		}
		out := solana.PublicKeyFromBytes(*v)
		return append(into, out.Bytes()...), nil
	case solana.PublicKey:
		return append(into, v.Bytes()...), nil
	default:
		return nil, fmt.Errorf("unknown type %T provided for encoding solana public key", value)
	}
}

func (p PublicKey) Decode(encoded []byte) (any, []byte, error) {
	return solana.PublicKeyFromBytes(encoded), nil, nil
}

func (p PublicKey) GetType() reflect.Type {
	// Pointer type so that nil can inject values and so that the NamedCodec won't wrap with no-nil pointer.
	return reflect.TypeOf(&[]byte{})
}

func (p PublicKey) Size(_ int) (int, error) {
	return solana.PublicKeyLength, nil
}

func (p PublicKey) FixedSize() (int, error) {
	return solana.PublicKeyLength, nil
}
