package types_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codecv2"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func TestCreateEventIdlWrapper_WrapperValueMethods(t *testing.T) {
	// Parse v1 IDL and create CodecEventIdl wrapper
	var codecIDLv1 codec.IDL
	idlString := codec.FetchLogpollerTypeTestIDL()
	err := json.Unmarshal([]byte(idlString), &codecIDLv1)
	require.NoError(t, err)
	eventIdlv1, err := codec.ExtractEventIDL("TestItem", codecIDLv1)
	require.NoError(t, err)
	codecEventIDL := logpollertypes.CodecEventIdl{Event: eventIdlv1, Types: codecIDLv1.Types}
	var wrapperV1 logpollertypes.EventIdlWrapper
	wrapperV1.Set(&codecEventIDL)

	// Parse v2 IDL and create Codecv2EventIdl wrapper
	var codecIDLv2 anchoridl.Idl
	err = json.Unmarshal([]byte(codecv2.FetchLogpollerTypeTestIDL()), &codecIDLv2)
	require.NoError(t, err)
	eventIdlv2, err := codecv2.ExtractEventIDL("TestItem", codecIDLv2)
	require.NoError(t, err)
	codecv2EventIDL := logpollertypes.Codecv2EventIdl{Event: eventIdlv2, Types: codecIDLv2.Types}
	var wrapperV2 logpollertypes.EventIdlWrapper
	wrapperV2.Set(&codecv2EventIDL)
	t.Run("Value() returns nil for empty wrapper", func(t *testing.T) {
		var wrapper logpollertypes.EventIdlWrapper
		value, err := wrapper.Value()
		require.NoError(t, err)
		require.Nil(t, value)
	})

	t.Run("Value() returns JSON for codecv2 wrapper", func(t *testing.T) {
		value, err := wrapperV2.Value()
		require.NoError(t, err)
		require.NotNil(t, value)

		// Should be valid JSON bytes
		jsonBytes, ok := value.([]byte)
		require.True(t, ok)
		require.NotEmpty(t, jsonBytes)
	})

	t.Run("Value() returns JSON for codec v1 wrapper", func(t *testing.T) {
		value, err := wrapperV1.Value()
		require.NoError(t, err)
		require.NotNil(t, value)

		// Should be valid JSON bytes
		jsonBytes, ok := value.([]byte)
		require.True(t, ok)
		require.NotEmpty(t, jsonBytes)
	})

	t.Run("Scan() and Get() round-trip for codecv2", func(t *testing.T) {
		// Get the value (JSON bytes)
		value, err := wrapperV2.Value()
		require.NoError(t, err)

		// Scan into a new wrapper
		var newWrapper logpollertypes.EventIdlWrapper
		err = newWrapper.Scan(value)
		require.NoError(t, err)

		// Verify they're equal
		originalIdl := wrapperV2.Get()
		newIdl := newWrapper.Get()
		require.NotNil(t, originalIdl)
		require.NotNil(t, newIdl)

		// Type check: original should be Codecv2EventIdl, scanned should also be Codecv2EventIdl
		_, originalIsV2 := originalIdl.(*logpollertypes.Codecv2EventIdl)
		require.True(t, originalIsV2, "Original v2 wrapper should contain Codecv2EventIdl")
		_, scannedIsV2 := newIdl.(*logpollertypes.Codecv2EventIdl)
		require.True(t, scannedIsV2, "Scanned v2 wrapper should contain Codecv2EventIdl, not CodecEventIdl")

		require.True(t, originalIdl.Equal(newIdl))
	})

	t.Run("Scan() and Get() round-trip for codec v1", func(t *testing.T) {
		// Get the value (JSON bytes)
		value, err := wrapperV1.Value()
		require.NoError(t, err)

		// Scan into a new wrapper
		var newWrapper logpollertypes.EventIdlWrapper
		err = newWrapper.Scan(value)
		require.NoError(t, err)

		// Verify they're equal
		originalIdl := wrapperV1.Get()
		newIdl := newWrapper.Get()
		require.NotNil(t, originalIdl)
		require.NotNil(t, newIdl)

		// Type check: original should be CodecEventIdl, scanned should also be CodecEventIdl
		_, originalIsV1 := originalIdl.(*logpollertypes.CodecEventIdl)
		require.True(t, originalIsV1, "Original v1 wrapper should contain CodecEventIdl")
		_, scannedIsV1 := newIdl.(*logpollertypes.CodecEventIdl)
		require.True(t, scannedIsV1, "Scanned v1 wrapper should contain CodecEventIdl, not Codecv2EventIdl")

		require.True(t, originalIdl.Equal(newIdl))
	})
}
