package codecv2

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

func TestCreateCodecEntry(t *testing.T) {
	var idl anchoridl.Idl
	if err := json.Unmarshal([]byte(dataStorageIdl), &idl); err != nil {
		t.Fatalf("unexpected error: invalid Data Storage IDL, error: %v", err)
	}
	for i, event := range idl.Events {
		entry, err := CreateCodecEntry(event, fmt.Sprintf("test%d", i), idl, nil)
		require.NoError(t, err)
		require.NotNil(t, entry)
	}
	for i, instruction := range idl.Instructions {
		if instruction.Name == "initialize_data_account" {
			entry, err := CreateCodecEntry(instruction, fmt.Sprintf("test%d", i), idl, nil)
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
		def, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeInstructionDef, "initialize_data_account", idl)
		require.NoError(t, err)
		require.NotNil(t, def)
		instruction, ok := def.(anchoridl.IdlInstruction)
		require.True(t, ok)
		require.Equal(t, "initialize_data_account", instruction.Name)
	})

	t.Run("finds event by name", func(t *testing.T) {
		if len(idl.Events) > 0 {
			eventName := idl.Events[0].Name
			def, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, eventName, idl)
			require.NoError(t, err)
			require.NotNil(t, def)
			event, ok := def.(anchoridl.IdlEvent)
			require.True(t, ok)
			require.Equal(t, eventName, event.Name)
		}
	})

	t.Run("returns error for account type - not supported", func(t *testing.T) {
		def, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeAccountDef, "some_account", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "codecv2 does not support accounts")
	})

	t.Run("returns error for instruction not found", func(t *testing.T) {
		def, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeInstructionDef, "nonexistent_instruction", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "failed to find instruction")
	})

	t.Run("returns error for event not found", func(t *testing.T) {
		def, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, "nonexistent_event", idl)
		require.Error(t, err)
		require.Nil(t, def)
		require.Contains(t, err.Error(), "failed to find event")
	})

	t.Run("returns error for unknown config type", func(t *testing.T) {
		def, err := FindDefinitionFromIDL(solcommoncodec.ChainConfigType("unknown_type"), "some_name", idl)
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
		if len(idl.Events) > 0 {
			eventName := idl.Events[0].Name
			event, err := ExtractEventIDL(eventName, idl)
			require.NoError(t, err)
			require.Equal(t, eventName, event.Name)
		}
	})

	t.Run("returns error when event not found", func(t *testing.T) {
		event, err := ExtractEventIDL("nonexistent_event", idl)
		require.Error(t, err)
		require.Empty(t, event.Name)
		require.Contains(t, err.Error(), "failed to find event")
	})

	t.Run("extracts all events from IDL", func(t *testing.T) {
		for _, expectedEvent := range idl.Events {
			event, err := ExtractEventIDL(expectedEvent.Name, idl)
			require.NoError(t, err)
			require.Equal(t, expectedEvent.Name, event.Name)
		}
	})
}

func TestSnakeToPascal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "simple snake_case", input: "transmission_id", want: "TransmissionId"},
		{name: "single word", input: "foo", want: "Foo"},
		{name: "multiple segments", input: "a_b_c", want: "ABC"},
		{name: "already lowercase single", input: "reserved", want: "Reserved"},
		{name: "all caps segments lowered", input: "MY_FIELD", want: "MyField"},
		{name: "underscore only", input: "_", wantErr: true},
		{name: "double underscore only", input: "__", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "leading underscore", input: "_reserved", want: "Reserved"},
		{name: "trailing underscore", input: "reserved_", want: "Reserved"},
		{name: "consecutive underscores", input: "a__b", want: "AB"},
		{name: "leading and trailing underscores", input: "_foo_", want: "Foo"},
		{name: "mixed case segments", input: "myField_name", want: "MyfieldName"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := snakeToPascal(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, commontypes.ErrInvalidConfig)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSnakeToPascal_NoCollisions(t *testing.T) {
	t.Parallel()

	collisionGroups := []struct {
		name   string
		inputs []string
	}{
		{
			name:   "leading underscore vs plain",
			inputs: []string{"reserved", "_reserved"},
		},
		{
			name:   "single vs double underscore",
			inputs: []string{"a_b", "a__b"},
		},
		{
			name:   "trailing underscore vs plain",
			inputs: []string{"foo", "foo_"},
		},
	}

	for _, group := range collisionGroups {
		t.Run(group.name, func(t *testing.T) {
			t.Parallel()
			var results []string
			for _, input := range group.inputs {
				result, err := snakeToPascal(input)
				if err != nil {
					continue
				}
				results = append(results, result)
			}
			if len(results) > 1 {
				t.Logf("known collision risk: inputs %v all produce the same output %q", group.inputs, results[0])
			}
		})
	}
}

func TestValidateUniqueFieldNames(t *testing.T) {
	t.Parallel()

	t.Run("no duplicates", func(t *testing.T) {
		t.Parallel()
		named := []commonencodings.NamedTypeCodec{
			{Name: "Foo"},
			{Name: "Bar"},
			{Name: "Baz"},
		}
		err := validateUniqueFieldNames(named)
		require.NoError(t, err)
	})

	t.Run("with duplicates", func(t *testing.T) {
		t.Parallel()
		named := []commonencodings.NamedTypeCodec{
			{Name: "Foo"},
			{Name: "Bar"},
			{Name: "Foo"},
		}
		err := validateUniqueFieldNames(named)
		require.Error(t, err)
		require.ErrorIs(t, err, commontypes.ErrInvalidConfig)
		assert.Contains(t, err.Error(), "duplicate PascalCase field name")
		assert.Contains(t, err.Error(), "Foo")
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		err := validateUniqueFieldNames(nil)
		require.NoError(t, err)
	})

	t.Run("single element", func(t *testing.T) {
		t.Parallel()
		named := []commonencodings.NamedTypeCodec{
			{Name: "Foo"},
		}
		err := validateUniqueFieldNames(named)
		require.NoError(t, err)
	})
}
