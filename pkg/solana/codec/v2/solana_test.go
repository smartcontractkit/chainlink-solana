package codecv2_test

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/stretchr/testify/require"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
)

func TestCreateCodecEntry(t *testing.T) {
	var idl anchoridl.Idl
	if err := json.Unmarshal([]byte(dataStorageIdl), &idl); err != nil {
		t.Fatalf("unexpected error: invalid Data Storage IDL, error: %v", err)
	}
	for i, event := range idl.Events {
		entry, err := codecv2.CreateCodecEntry(event, fmt.Sprintf("test%d", i), idl, nil)
		require.NoError(t, err)
		require.NotNil(t, entry)
	}
	for i, instruction := range idl.Instructions {
		if instruction.Name == "initialize_data_account" {
			entry, err := codecv2.CreateCodecEntry(instruction, fmt.Sprintf("test%d", i), idl, nil)
			require.NoError(t, err)
			require.NotNil(t, entry)
		}
	}
}

func TestFindDefinitionFromIDL(t *testing.T) {
	var idl anchoridl.Idl
	err := json.Unmarshal([]byte(dataStorageIdl), &idl)
	require.NoError(t, err)

	t.Run("finds instruction by name", func(t *testing.T) {
		def, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeInstructionDef, "initialize_data_account", idl)
		require.NoError(t, err)
		require.NotNil(t, def)
		instruction, ok := def.(anchoridl.IdlInstruction)
		require.True(t, ok)
		require.Equal(t, "initialize_data_account", instruction.Name)
	})

	t.Run("finds event by name", func(t *testing.T) {
		// Assuming the IDL has at least one event
		if len(idl.Events) > 0 {
			eventName := idl.Events[0].Name
			def, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, eventName, idl)
			require.NoError(t, err)
			require.NotNil(t, def)
			event, ok := def.(anchoridl.IdlEvent)
			require.True(t, ok)
			require.Equal(t, eventName, event.Name)
		}
	})

	t.Run("returns error for account type - not supported", func(t *testing.T) {
		def, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeAccountDef, "some_account", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "codecv2 does not support accounts")
	})

	t.Run("returns error for instruction not found", func(t *testing.T) {
		def, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeInstructionDef, "nonexistent_instruction", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "failed to find instruction")
	})

	t.Run("returns error for event not found", func(t *testing.T) {
		def, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, "nonexistent_event", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "failed to find event")
	})

	t.Run("returns error for unknown config type", func(t *testing.T) {
		def, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigType("unknown_type"), "some_name", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "unknown type")
	})
}

func TestExtractEventIDL(t *testing.T) {
	var idl anchoridl.Idl
	err := json.Unmarshal([]byte(dataStorageIdl), &idl)
	require.NoError(t, err)

	t.Run("successfully extracts event by name", func(t *testing.T) {
		// Assuming the IDL has at least one event
		if len(idl.Events) > 0 {
			eventName := idl.Events[0].Name
			event, err := codecv2.ExtractEventIDL(eventName, idl)
			require.NoError(t, err)
			require.Equal(t, eventName, event.Name)
		}
	})

	t.Run("returns error when event not found", func(t *testing.T) {
		event, err := codecv2.ExtractEventIDL("nonexistent_event", idl)
		require.Error(t, err)
		require.Empty(t, event.Name)
		require.Contains(t, err.Error(), "failed to find event")
	})

	t.Run("extracts all events from IDL", func(t *testing.T) {
		// Test that we can extract all events in the IDL
		for _, expectedEvent := range idl.Events {
			event, err := codecv2.ExtractEventIDL(expectedEvent.Name, idl)
			require.NoError(t, err)
			require.Equal(t, expectedEvent.Name, event.Name)
		}
	})
}
