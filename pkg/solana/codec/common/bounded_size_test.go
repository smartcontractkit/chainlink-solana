package commoncodec_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"

	commoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

func heapAllocMB() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	return stats.HeapAlloc >> 20
}

// oversizedLengthPrefix is a 4 byte Borsh length claiming math.MaxInt32 elements,
// followed by a single byte of payload.
var oversizedLengthPrefix = []byte{0xFF, 0xFF, 0xFF, 0x7F, 0x01}

func TestNewBoundedSlice_RejectsOversizedLengthWithoutAllocating(t *testing.T) {
	t.Parallel()

	builder := binary.LittleEndian()
	uint64Codec, err := builder.Uint(8)
	require.NoError(t, err)
	arrayCodec, err := encodings.NewArray(32, builder.Uint8())
	require.NoError(t, err)

	// a nested vector is only a 4 byte length prefix on the wire but a 24 byte
	// slice header in memory, so it must be bounded on the in-memory footprint
	nestedCodec, err := commoncodec.NewBoundedSlice(builder.Uint8(), builder)
	require.NoError(t, err)

	for _, tt := range []struct {
		name     string
		element  encodings.TypeCodec
		maxCount int
	}{
		{"vec of u8", builder.Uint8(), commoncodec.MaxAccountBytes},
		{"vec of u64", uint64Codec, commoncodec.MaxAccountBytes / 8},
		{"vec of [u8; 32]", arrayCodec, commoncodec.MaxAccountBytes / 32},
		{"vec of vec of u8", nestedCodec, commoncodec.MaxAccountBytes / 24},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			codec, err := commoncodec.NewBoundedSlice(tt.element, builder)
			require.NoError(t, err)

			before := heapAllocMB()
			_, _, err = codec.Decode(oversizedLengthPrefix)
			after := heapAllocMB()

			require.ErrorContains(t, err, "exceeds the maximum")
			require.ErrorContains(t, err, "2147483647")
			require.ErrorContains(t, err, fmt.Sprintf("maximum of %d", tt.maxCount))
			// the wire length must be rejected before reflect.MakeSlice runs
			require.Less(t, after-before, uint64(16), "decode allocated %d MB", after-before)

			// whatever the cap is, admitting that many elements must stay within
			// MaxAccountBytes of memory, not just of wire bytes
			elementBytes := int(tt.element.GetType().Size())
			require.LessOrEqual(t, tt.maxCount*elementBytes, commoncodec.MaxAccountBytes,
				"cap of %d elements of %d bytes exceeds the memory budget", tt.maxCount, elementBytes)
		})
	}
}

func TestNewBoundedSlice_WireFormatUnchanged(t *testing.T) {
	t.Parallel()

	builder := binary.LittleEndian()
	value := []byte{1, 2, 3, 4, 5}

	bounded, err := commoncodec.NewBoundedSlice(builder.Uint8(), builder)
	require.NoError(t, err)

	size, err := builder.Int(4)
	require.NoError(t, err)
	unbounded, err := encodings.NewSlice(builder.Uint8(), size)
	require.NoError(t, err)

	boundedBytes, err := bounded.Encode(value, nil)
	require.NoError(t, err)
	unboundedBytes, err := unbounded.Encode(value, nil)
	require.NoError(t, err)

	require.Equal(t, unboundedBytes, boundedBytes, "bounding the length prefix must not change the wire format")
	require.Equal(t, []byte{5, 0, 0, 0, 1, 2, 3, 4, 5}, boundedBytes)

	decoded, remaining, err := bounded.Decode(boundedBytes)
	require.NoError(t, err)
	require.Equal(t, value, decoded)
	require.Empty(t, remaining)
	require.Equal(t, unbounded.GetType(), bounded.GetType())
}

func TestNewBoundedSlice_AcceptsLengthAtLimit(t *testing.T) {
	t.Parallel()

	builder := binary.LittleEndian()
	codec, err := commoncodec.NewBoundedSlice(builder.Uint8(), builder)
	require.NoError(t, err)

	// a length exactly at the cap must pass the bound and fail only on short input,
	// proving the guard rejects on size rather than truncating valid payloads
	atLimit := []byte{0x00, 0x00, 0xA0, 0x00, 0x01}
	_, _, err = codec.Decode(atLimit)
	require.ErrorContains(t, err, "not enough bytes")
}

func TestCheckElementCount(t *testing.T) {
	t.Parallel()

	builder := binary.LittleEndian()
	uint64Codec, err := builder.Uint(8)
	require.NoError(t, err)

	require.NoError(t, commoncodec.CheckElementCount(32, builder.Uint8()))
	require.NoError(t, commoncodec.CheckElementCount(commoncodec.MaxAccountBytes, builder.Uint8()))

	require.ErrorContains(t, commoncodec.CheckElementCount(commoncodec.MaxAccountBytes+1, builder.Uint8()), "exceeds the maximum")
	require.ErrorContains(t, commoncodec.CheckElementCount(commoncodec.MaxAccountBytes, uint64Codec), "exceeds the maximum")
	require.ErrorContains(t, commoncodec.CheckElementCount(-1, builder.Uint8()), "exceeds the maximum")
	require.ErrorContains(t, commoncodec.CheckElementCount(1, nil), "must be non-nil")
}
