package config_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

//go:embed testChainReader_valid.json
var validJSON string

//go:embed testChainReader_invalid.json
var invalidJSON string

func TestChainReaderConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid unmarshal", func(t *testing.T) {
		t.Parallel()

		var result config.ChainReader
		require.NoError(t, json.Unmarshal([]byte(validJSON), &result))
		assert.Equal(t, validChainReaderConfig, result)
	})

	t.Run("invalid unmarshal", func(t *testing.T) {
		t.Parallel()

		var result config.ChainReader
		require.ErrorIs(t, json.Unmarshal([]byte(invalidJSON), &result), types.ErrInvalidConfig)
	})

	t.Run("marshal", func(t *testing.T) {
		t.Parallel()

		result, err := json.Marshal(validChainReaderConfig)

		require.NoError(t, err)

		var conf config.ChainReader

		require.NoError(t, json.Unmarshal(result, &conf))
		assert.Equal(t, validChainReaderConfig, conf)
	})
}

func TestEncodingType_Fail(t *testing.T) {
	t.Parallel()

	_, err := json.Marshal(config.EncodingType(100))

	require.NotNil(t, err)

	var tp config.EncodingType

	require.ErrorIs(t, json.Unmarshal([]byte(`42`), &tp), types.ErrInvalidConfig)
	require.ErrorIs(t, json.Unmarshal([]byte(`"invalid"`), &tp), types.ErrInvalidConfig)
}

func TestBuilderForEncoding_Default(t *testing.T) {
	t.Parallel()

	builder := config.BuilderForEncoding(config.EncodingType(100))
	require.Equal(t, binary.LittleEndian(), builder)
}

var (
	encodingBase64 = solana.EncodingBase64
	commitment     = rpc.CommitmentFinalized
	offset         = uint64(10)
	length         = uint64(10)
)

var validChainReaderConfig = config.ChainReader{
	Namespaces: map[string]config.ChainReaderMethods{
		"Contract": {
			Methods: map[string]config.ChainDataReader{
				"Method": {
					AnchorIDL: "test idl 1",
					Encoding:  config.EncodingTypeBorsh,
					Procedures: []config.ChainReaderProcedure{
						{
							IDLAccount: testutils.TestStructWithNestedStruct,
						},
					},
				},
				"MethodWithOpts": {
					AnchorIDL: "test idl 2",
					Encoding:  config.EncodingTypeBorsh,
					Procedures: []config.ChainReaderProcedure{
						{
							IDLAccount: testutils.TestStructWithNestedStruct,
							OutputModifications: codeccommon.ModifiersConfig{
								&codeccommon.PropertyExtractorConfig{FieldName: "DurationVal"},
							},
							RPCOpts: &config.RPCOpts{
								Encoding:   &encodingBase64,
								Commitment: &commitment,
								DataSlice: &rpc.DataSlice{
									Offset: &offset,
									Length: &length,
								},
							},
						},
					},
				},
			},
		},
		"OtherContract": {
			Methods: map[string]config.ChainDataReader{
				"Method": {
					AnchorIDL: "test idl 3",
					Encoding:  config.EncodingTypeBincode,
					Procedures: []config.ChainReaderProcedure{
						{
							IDLAccount: testutils.TestStructWithNestedStruct,
						},
					},
				},
			},
		},
	},
}
