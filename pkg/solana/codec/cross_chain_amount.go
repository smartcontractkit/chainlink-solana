package codec

import (
	"fmt"
	"math/big"
	"reflect"

	cciptypes "github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func NewCrossChainAmount(builder encodings.Builder) encodings.TypeCodec {
	return &crossChainAmount{}
}

type crossChainAmount struct {
	intEncoder encodings.TypeCodec
}

var _ encodings.TypeCodec = &crossChainAmount{}

func (d *crossChainAmount) Encode(value any, into []byte) ([]byte, error) {
	bi, ok := value.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("%w: expected []byte, got %T", types.ErrInvalidType, value)
	}

	bytes := encodeBigIntToFixedLengthLE(bi, 32)
	return append(into, bytes...), nil
}

func (d *crossChainAmount) Decode(encoded []byte) (any, []byte, error) {
	// TODO: assert >= 32 remaining
	buf := encoded[0:32]
	encoded = encoded[32:]

	bi := decodeLEToBigInt(buf)
	return bi, encoded, nil
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

func decodeLEToBigInt(data []byte) cciptypes.BigInt {
	// Reverse the byte array to convert it from little-endian to big-endian
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}

	// Use big.Int.SetBytes to construct the big.Int
	bi := new(big.Int).SetBytes(data)
	if bi.Cmp(big.NewInt(0)) == 0 {
		return cciptypes.NewBigInt(big.NewInt(0))
	}

	return cciptypes.NewBigInt(bi)
}

func (d *crossChainAmount) GetType() reflect.Type {
	return reflect.TypeOf(cciptypes.BigInt{})
}

func (d *crossChainAmount) Size(val int) (int, error) {
	return 32, nil
}

func (d *crossChainAmount) FixedSize() (int, error) {
	return 32, nil
}
