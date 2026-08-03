package commoncodec

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
)

// IndexedValue represents a value which can be written to, read from, or compared to an indexed BYTEA
// postgres field. Maps, structs, and slices or arrays (of anything but byte) are not supported. For signed
// or unsigned integer types, strings, or byte arrays, the SQL operators <, =, & > should work in the expected
// way.
type IndexedValue []byte

func (v *IndexedValue) FromUint64(u uint64) {
	*v = make([]byte, 8)
	binary.BigEndian.PutUint64(*v, u)
}

func (v *IndexedValue) FromInt64(i int64) {
	// Golang signed integers are two's complement encoded, so the value will be stored with two's complement encoding.
	// This also matches the EVM implementation that stores raw topics, EVM ABI encoding also uses two's complement
	v.FromUint64(uint64(i)) // nolint gosec two's complement encoding
}

func (v *IndexedValue) FromFloat64(f float64) {
	if f > 0 {
		v.FromUint64(math.Float64bits(f) + math.MaxInt64 + 1)
		return
	}
	v.FromUint64(math.MaxInt64 + 1 - math.Float64bits(f))
}

func (v *IndexedValue) FromBool(b bool) {
	if b {
		*v = []byte{1}
		return
	}
	*v = []byte{0}
}

// NewIndexedValue builds an IndexedValue from a decoded Go value (as produced by ExtractField).
// Supported kinds: []byte, string, bool, any signed/unsigned integer or float type, and byte arrays
// of any length (e.g. [32]byte public keys).
func NewIndexedValue(typedVal any) (iVal IndexedValue, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v while creating indexedValue for %T", r, typedVal)
		}
	}()
	// handle simplest cases first
	switch t := typedVal.(type) {
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	case bool:
		iVal.FromBool(t)
		return iVal, nil
	}

	// handle numeric types
	v := reflect.ValueOf(typedVal)
	if v.CanUint() {
		iVal.FromUint64(v.Uint())
		return iVal, nil
	}
	if v.CanInt() {
		iVal.FromInt64(v.Int())
		return iVal, nil
	}
	if v.CanFloat() {
		iVal.FromFloat64(v.Float())
		return iVal, nil
	}

	// any length array is fine as long as the element type is byte
	if t := v.Type(); t.Kind() == reflect.Array {
		if t.Elem().Kind() == reflect.Uint8 {
			if v.CanAddr() {
				return v.Bytes(), nil
			}
			result := make([]byte, v.Len())
			l := v.Len()
			for i := 0; i < l; i++ {
				result[i] = byte(v.Index(i).Uint())
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("can't create indexed value from type %T", typedVal)
}
