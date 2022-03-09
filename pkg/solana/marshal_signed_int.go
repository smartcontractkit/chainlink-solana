// code from: https://github.com/smartcontractkit/chainlink-terra/blob/develop/pkg/terra/marshal_signed_int.go
// will eventually be removed and replaced with a generalized version from libocr

package solana

import (
	"bytes"
	"fmt"
	"math/big"
)

var i = big.NewInt

func bounds(numBytes uint) (*big.Int, *big.Int) {
	max := i(0).Sub(i(0).Lsh(i(1), numBytes*8-1), i(1)) // 2**(numBytes*8-1)- 1
	min := i(0).Sub(i(0).Neg(max), i(1))                // -2**(numBytes*8-1)
	return min, max
}

// ToBigInt interprets bytes s as a big-endian signed integer
// of size numBytes.
func ToBigInt(s []byte, numBytes uint) (*big.Int, error) {
	if uint(len(s)) != numBytes {
		return nil, fmt.Errorf("invalid int length: expected %d got %d", numBytes, len(s))
	}
	val := (&big.Int{}).SetBytes(s)
	numBits := numBytes * 8
	_, max := bounds(numBytes)
	negative := val.Cmp(max) > 0
	if negative {
		// Get the complement wrt to 2^numBits
		maxUint := big.NewInt(1)
		maxUint.Lsh(maxUint, numBits)
		val.Sub(maxUint, val)
		val.Neg(val)
	}
	return val, nil
}

// ToBytes converts *big.Int o into bytes as a big-endian signed
// integer of size numBytes
func ToBytes(o *big.Int, numBytes uint) ([]byte, error) {
	min, max := bounds(numBytes)
	if o.Cmp(max) > 0 || o.Cmp(min) < 0 {
		return nil, fmt.Errorf("value won't fit in int%v: 0x%x", numBytes*8, o)
	}
	negative := o.Sign() < 0
	val := (&big.Int{})
	numBits := numBytes * 8
	if negative {
		// compute two's complement as 2**numBits - abs(o) = 2**numBits + o
		val.SetInt64(1)
		val.Lsh(val, numBits)
		val.Add(val, o)
	} else {
		val.Set(o)
	}
	b := val.Bytes() // big-endian representation of abs(val)
	if uint(len(b)) > numBytes {
		return nil, fmt.Errorf("b must fit in %v bytes", numBytes)
	}
	b = bytes.Join([][]byte{bytes.Repeat([]byte{0}, int(numBytes)-len(b)), b}, []byte{})
	if uint(len(b)) != numBytes {
		return nil, fmt.Errorf("wrong length; there must be an error in the padding of b: %v", b)
	}
	return b, nil
}
