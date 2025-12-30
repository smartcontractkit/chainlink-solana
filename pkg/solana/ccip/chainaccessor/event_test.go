package chainaccessor

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

//go:embed testdata/testIDL_v1.json
var testIDLv1 string

//go:embed testdata/testIDL_v2.json
var testIDLv2 string

func TestCreateEventIdlWrapper(t *testing.T) {
	t.Run("creates wrapper with codec v1 IDL", func(t *testing.T) {
		// testIDL_v1.json has a TestItem event with various field types
		wrapper, err := CreateEventIdlWrapper("TestItem", testIDLv1)
		require.NoError(t, err)

		// Verify the inner IDL is set
		innerIdl := wrapper.Get()
		require.NotNil(t, innerIdl)

		// Verify it's a CodecEventIdl (v1)
		codecIdl, ok := innerIdl.(*logpollertypes.CodecEventIdl)
		require.True(t, ok, "Expected CodecEventIdl type for v1 IDL")
		require.Equal(t, "TestItem", codecIdl.Event.Name)
		require.NotEmpty(t, codecIdl.Event.Fields)
		require.NotEmpty(t, codecIdl.Types)

		// Verify some of the fields
		require.Len(t, codecIdl.Event.Fields, 9) // TestItem has 9 fields
	})

	t.Run("creates wrapper with codec v2 IDL", func(t *testing.T) {
		wrapper, err := CreateEventIdlWrapper("TestItem", testIDLv2)
		require.NoError(t, err)

		// Verify the inner IDL is set
		innerIdl := wrapper.Get()
		require.NotNil(t, innerIdl)

		// Verify it's a Codecv2EventIdl
		codecv2Idl, ok := innerIdl.(*logpollertypes.Codecv2EventIdl)
		require.True(t, ok, "Expected Codecv2EventIdl type")
		require.Equal(t, "TestItem", codecv2Idl.Event.Name)
		require.Len(t, codecv2Idl.Event.Discriminator, 8)
		require.NotEmpty(t, codecv2Idl.Types)
	})

	t.Run("returns error for non-existent event in v1 IDL", func(t *testing.T) {
		wrapper, err := CreateEventIdlWrapper("NonExistentEvent", testIDLv1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to extract event IDL from codec")
		require.Contains(t, err.Error(), "failed to find event")

		// Wrapper should be empty
		require.Nil(t, wrapper.Get())
	})

	t.Run("returns error for non-existent event in v2 IDL", func(t *testing.T) {
		wrapper, err := CreateEventIdlWrapper("NonExistentEvent", testIDLv2)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to extract event IDL from codecv2")
		require.Contains(t, err.Error(), "failed to find event")

		// Wrapper should be empty
		require.Nil(t, wrapper.Get())
	})
}
