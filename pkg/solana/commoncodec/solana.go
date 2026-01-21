/*
Package codec provides functions to create a codec from an Anchor IDL. All Anchor primitives map to the following native
Go values:

bool -> bool
string -> string
bytes -> []byte
[u|i][8-64] -> [u]int[8-64]
[u|i]128 -> *big.Int
duration -> time.Duration
unixTimestamp -> int64
publicKey -> [32]byte
hash -> [32]byte

Enums as an Anchor data structure are only supported in their basic form of uint8 values. Enums with variants are not
supported at this time.

Modifiers can be provided to assist in modifying property names, adding properties, etc.
*/
package commoncodec

import (
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

const (
	DefaultHashBitLength = 32
	unknownIDLFormat     = "%w: unknown IDL type def %q"
)

// DecoderHooks
//
// BigIntHook allows *big.Int to be represented as any integer type or a string and to go back to them.
// Useful for config, or if when a model may use a go type that isn't a *big.Int when Pack expects one.
// Eg: int32 in a go struct from a plugin could require a *big.Int in Pack for int24, if it fits, we shouldn't care.
// SliceToArrayVerifySizeHook verifies that slices have the correct size when converting to an array
// EpochToTimeHook allows multiple conversions: time.Time -> int64; int64 -> time.Time; *big.Int -> time.Time; and more
var DecoderHooks = []mapstructure.DecodeHookFunc{commoncodec.EpochToTimeHook, commoncodec.BigIntHook, commoncodec.SliceToArrayVerifySizeHook}

type solanaCodec struct {
	commontypes.Encoder
	commontypes.Decoder
	*ParsedTypes
}

func (s solanaCodec) CreateType(itemType string, forEncoding bool) (any, error) {
	var itemTypes map[string]Entry
	if forEncoding {
		itemTypes = s.EncoderDefs
	} else {
		itemTypes = s.DecoderDefs
	}

	def, ok := itemTypes[itemType]
	if !ok {
		return nil, fmt.Errorf("%w: cannot find type name %q", commontypes.ErrInvalidType, itemType)
	}

	// we don't need double pointers, and they can also mess up reflection variable creation and mapstruct decode
	if def.GetType().Kind() == reflect.Pointer {
		return reflect.New(def.GetCodecType().GetType().Elem()).Interface(), nil
	}

	return reflect.New(def.GetType()).Interface(), nil
}

func WrapItemType(forEncoding bool, contractName, itemType string) string {
	if forEncoding {
		return fmt.Sprintf("input.%s.%s", contractName, itemType)
	}

	return fmt.Sprintf("output.%s.%s", contractName, itemType)
}
