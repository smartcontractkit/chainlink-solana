package commoncodec

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	cciptypes "github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
)

const CrossChainAmountLength = 32

func NewCrossChainAmount() encodings.TypeCodec {
	return &crossChainAmount{}
}

type crossChainAmount struct{}

var _ encodings.TypeCodec = &crossChainAmount{}

func (d *crossChainAmount) Encode(value any, into []byte) ([]byte, error) {
	var bi *big.Int
	switch v := value.(type) {
	case *big.Int:
		bi = v
	case cciptypes.BigInt:
		bi = v.Int
	default:
		return nil, fmt.Errorf("%w: expected big.Int, got %T", types.ErrInvalidType, value)
	}

	bytes := encodeBigIntToFixedLengthLE(bi, 32)
	return append(into, bytes...), nil
}

func (d *crossChainAmount) Decode(encoded []byte) (any, []byte, error) {
	decoded, remaining, err := encodings.SafeDecode(encoded, CrossChainAmountLength, func(raw []byte) cciptypes.BigInt {
		return DecodeLEToBigInt(raw)
	})
	if err != nil {
		return nil, nil, err
	}
	return decoded, remaining, nil
}

func encodeBigIntToFixedLengthLE(bi *big.Int, length int) []byte {
	// Create a fixed-length byte array
	paddedBytes := make([]byte, length)

	// Use FillBytes to fill the array with big-endian data, zero-padded
	bi.FillBytes(paddedBytes)

	// Reverse the array for little-endian encoding
	for i, j := 0, len(paddedBytes)-1; i < j; i, j = i+1, j-1 {
		paddedBytes[i], paddedBytes[j] = paddedBytes[j], paddedBytes[i]
	}

	return paddedBytes
}

func DecodeLEToBigInt(data []byte) cciptypes.BigInt {
	// Avoid modifying original data
	buf := make([]byte, len(data))
	copy(buf, data)

	// Reverse the byte array to convert it from little-endian to big-endian
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	// Use big.Int.SetBytes to construct the big.Int
	bi := new(big.Int).SetBytes(buf)
	if bi.Cmp(big.NewInt(0)) == 0 {
		return cciptypes.NewBigInt(big.NewInt(0))
	}

	return cciptypes.NewBigInt(bi)
}

func (d *crossChainAmount) GetType() reflect.Type {
	return reflect.TypeOf(cciptypes.BigInt{})
}

func (d *crossChainAmount) Size(val int) (int, error) {
	return CrossChainAmountLength, nil
}

func (d *crossChainAmount) FixedSize() (int, error) {
	return CrossChainAmountLength, nil
}
