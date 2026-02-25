package config_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

//go:embed test_contract_reader_invalid.json
var invalidJSON string

//go:embed test_contract_reader_invalid_address_share_group.json
var invalidAddressShareGroupsJSON string

//go:embed test_contract_reader_valid.json
var validJSON string

//go:embed test_contract_reader_valid_with_IDL_as_string.json
var validJSONWithIDLAsString string

//go:embed test_contract_reader_valid_address_share_groups.json
var validJSONWithAddressShareGroups string

func TestChainReaderConfig(t *testing.T) {
	t.Parallel()

	t.Run("invalid unmarshal", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.ErrorIs(t, json.Unmarshal([]byte(invalidJSON), &result), types.ErrInvalidConfig)
	})

	t.Run("invalid address share group unmarshal", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.ErrorIs(t, json.Unmarshal([]byte(invalidAddressShareGroupsJSON), &result), types.ErrInvalidConfig)
	})

	t.Run("valid unmarshal with idl as struct", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.NoError(t, json.Unmarshal([]byte(validJSON), &result))
		assert.Equal(t, validChainReaderConfig, result)
	})

	t.Run("valid unmarshal with idl as string", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.NoError(t, json.Unmarshal([]byte(validJSONWithIDLAsString), &result))
		assert.Equal(t, validChainReaderConfig, result)
	})

	t.Run("valid unmarshal with PDA account", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.NoError(t, json.Unmarshal([]byte(validJSONWithIDLAsString), &result))
		assert.Equal(t, validChainReaderConfig, result)
	})

	t.Run("valid unmarshal with address share groups", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.NoError(t, json.Unmarshal([]byte(validJSONWithAddressShareGroups), &result))
		assert.Equal(t, validChainReaderConfigWithAddressShareGroups, result)
	})

	t.Run("marshal", func(t *testing.T) {
		t.Parallel()

		result, err := json.Marshal(validChainReaderConfig)
		require.NoError(t, err)

		var conf config.ContractReader
		require.NoError(t, json.Unmarshal(result, &conf))
		assert.Equal(t, validChainReaderConfig, conf)
	})
}

var nilIDL = codecv1.IDL{
	Version: "0.1.0",
	Name:    "myProgram",
	Accounts: codecv1.IdlTypeDefSlice{
		{Name: "NilType", Type: codecv1.IdlTypeDefTy{Kind: codecv1.IdlTypeDefTyKindStruct, Fields: &codecv1.IdlTypeDefStruct{}}},
	},
}

var validChainReaderConfig = config.ContractReader{
	Namespaces: map[string]config.ChainContractReader{
		"Contract": {
			IDL: nilIDL,
			Reads: map[string]config.ReadDefinition{
				"Method": {
					ChainSpecificName: testutils.TestStructWithNestedStruct,
				},
				"MethodWithOpts": {
					ChainSpecificName: testutils.TestStructWithNestedStruct,
					OutputModifications: codeccommon.ModifiersConfig{
						&codeccommon.PropertyExtractorConfig{FieldName: "DurationVal"},
					},
				},
			},
		},
		"OtherContract": {
			IDL: nilIDL,
			Reads: map[string]config.ReadDefinition{
				"Method": {
					ChainSpecificName: testutils.TestStructWithNestedStruct,
				},
			},
		},
	},
}

var validChainReaderConfigWithAddressShareGroups = config.ContractReader{
	AddressShareGroups: [][]string{{"a", "b", "c"}, {"u", "v", "w"}},
	Namespaces: map[string]config.ChainContractReader{
		"Contract": {
			IDL: nilIDL,
			Reads: map[string]config.ReadDefinition{
				"Method": {},
			},
		},
	},
}
