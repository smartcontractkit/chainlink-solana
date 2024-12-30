package config_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

//go:embed testChainReader_valid.json
var validJSON string

//go:embed testChainReader_valid_with_IDL_as_string.json
var validJSONWithIDLAsString string

//go:embed testChainReader_invalid.json
var invalidJSON string

func TestChainReaderConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid unmarshal with idl as struct", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.NoError(t, json.Unmarshal([]byte(validJSON), &result))
		assert.Equal(t, validChainReaderConfig, result)
	})

	t.Run("valid unmarshal with idl as string", func(t *testing.T) {
		var result config.ContractReader
		require.NoError(t, json.Unmarshal([]byte(validJSONWithIDLAsString), &result))
		assert.Equal(t, validChainReaderConfig, result)
	})

	t.Run("invalid unmarshal", func(t *testing.T) {
		t.Parallel()

		var result config.ContractReader
		require.ErrorIs(t, json.Unmarshal([]byte(invalidJSON), &result), types.ErrInvalidConfig)
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

var nilIDL = codec.IDL{
	Version: "0.1.0",
	Name:    "myProgram",
	Accounts: codec.IdlTypeDefSlice{
		{Name: "NilType", Type: codec.IdlTypeDefTy{Kind: codec.IdlTypeDefTyKindStruct, Fields: &codec.IdlTypeDefStruct{}}},
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
