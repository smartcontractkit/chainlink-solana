package codec

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type testErrDecodeEntry struct {
	entry
}

func (t *testErrDecodeEntry) Decode(_ []byte) (interface{}, []byte, error) {
	return nil, nil, fmt.Errorf("decode error")
}

type testErrDecodeRemainingBytes struct {
	entry
}

func (t *testErrDecodeRemainingBytes) Decode(_ []byte) (interface{}, []byte, error) {
	return struct{}{}, []byte{1}, nil
}

func TestDecoder_Decode_Errors(t *testing.T) {
	var into interface{}
	someType := "input.Some.Type"
	t.Run("error when item type not found", func(t *testing.T) {
		nonExistentType := "output.Non.Existent"
		err := newDecoder(map[string]Entry{someType: &entry{}}).
			Decode(tests.Context(t), []byte{}, &into, nonExistentType)
		require.ErrorIs(t, err, fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, nonExistentType))
	})

	t.Run("error when underlying entry decode fails", func(t *testing.T) {
		require.Error(t, newDecoder(map[string]Entry{someType: &testErrDecodeEntry{}}).
			Decode(tests.Context(t), []byte{}, &into, someType))
	})

	t.Run("remaining bytes exist after decode is ok", func(t *testing.T) {
		require.NoError(t, newDecoder(map[string]Entry{someType: &testErrDecodeRemainingBytes{}}).
			Decode(tests.Context(t), []byte{}, &into, someType))
	})
}

type testErrGetMaxDecodingSize struct {
	entry
}

type testErrGetMaxDecodingSizeCodecType struct {
	commonencodings.Empty
}

func (t testErrGetMaxDecodingSizeCodecType) Size(_ int) (int, error) {
	return 0, fmt.Errorf("error")
}

func (t *testErrGetMaxDecodingSize) GetCodecType() commonencodings.TypeCodec {
	return testErrGetMaxDecodingSizeCodecType{}
}

func TestDecoder_GetMaxDecodingSize_Errors(t *testing.T) {
	someType := "some-type"

	t.Run("error when entry for item type is missing", func(t *testing.T) {
		nonExistentType := "non-existent"
		_, err := newDecoder(map[string]Entry{someType: &entry{}}).
			GetMaxDecodingSize(tests.Context(t), 0, nonExistentType)
		require.ErrorIs(t, err, fmt.Errorf("%w: cannot find type %s", commontypes.ErrInvalidType, nonExistentType))
	})

	t.Run("error when underlying entry decode fails", func(t *testing.T) {
		_, err := newDecoder(map[string]Entry{someType: &testErrGetMaxDecodingSize{}}).
			GetMaxDecodingSize(tests.Context(t), 0, someType)
		require.Error(t, err)
	})
}

type UnknownAddress []byte

type Bytes32 [32]byte

type MerkleRoot struct {
	SourceChainSelector uint64
	OnRampAddress       UnknownAddress
	MinSeqNr            uint64
	MaxSeqNr            uint64
	MerkleRoot          Bytes32
}

type TokenPriceUpdate struct {
	SourceToken UnknownAddress
	UsdPerToken *big.Int
}

type GasPriceUpdate struct {
	DestChainSelector uint64
	UsdPerUnitGas     *big.Int
}

type PriceUpdates struct {
	TokenPriceUpdates []TokenPriceUpdate
	GasPriceUpdates   []GasPriceUpdate
}

type CommitReportAcceptedEvent struct {
	BlessedMerkleRoots   []MerkleRoot
	UnblessedMerkleRoots []MerkleRoot
	PriceUpdates         PriceUpdates
}

func BenchmarkDecode(b *testing.B) {
	b.StopTimer()

	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	// create a new codec
	rmCodec := setupCodec(b)
	encoded, err := rmCodec.Encode(ctx, createTestStruct(b), WrapItemType(true, "Contract", "CommitReportAcceptedEvent"))

	require.NoError(b, err)

	b.StartTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var into CommitReportAcceptedEvent

		require.NoError(b, rmCodec.Decode(ctx, encoded, &into, WrapItemType(false, "Contract", "CommitReportAcceptedEvent")))
	}
}

type TestingT interface {
	require.TestingT
	Helper()
}

func setupCodec(t TestingT) commontypes.RemoteCodec {
	t.Helper()

	// create an IDL that defines the expected encoded type
	var idl IDL
	require.NoError(t, json.Unmarshal([]byte(benchmarkIDL), &idl))

	def, err := FindDefinitionFromIDL(ChainConfigTypeEventDef, "CommitReportAcceptedEvent", idl)

	require.NoError(t, err)

	mods := commoncodec.MultiModifier{
		commoncodec.NewConstrainedLengthBytesToStringModifier([]string{"BlessedMerkleRoots.MerkleRoot"}, 32),
	}

	entry, err := CreateCodecEntry(def, "CommitReportAcceptedEvent", idl, mods)

	require.NoError(t, err)

	parsed := &ParsedTypes{
		EncoderDefs: map[string]Entry{WrapItemType(true, "Contract", "CommitReportAcceptedEvent"): entry},
		DecoderDefs: map[string]Entry{WrapItemType(false, "Contract", "CommitReportAcceptedEvent"): entry},
	}

	rmCodec, err := parsed.ToCodec()

	require.NoError(t, err)

	return rmCodec
}

func createTestStruct(t TestingT) CommitReportAcceptedEvent {
	t.Helper()

	return CommitReportAcceptedEvent{
		BlessedMerkleRoots:   []MerkleRoot{},
		UnblessedMerkleRoots: []MerkleRoot{},
		PriceUpdates: PriceUpdates{
			TokenPriceUpdates: []TokenPriceUpdate{},
			GasPriceUpdates: []GasPriceUpdate{
				{789068866484373046, big.NewInt(40000000028000)},
				{909606746561742123, big.NewInt(40000000028000)},
				{3379446385462418246, big.NewInt(4000000000000)},
				{5721565186521185178, big.NewInt(40000000028000)},
				{12922642891491394802, big.NewInt(40000000028000)},
			},
		},
	}
}

const benchmarkIDL = `
{
	"version": "0.1.0",
	"name": "benchmark_idl",
	"types": [
		{
			"name": "MerkleRoot",
			"type": {
				"kind": "struct",
				"fields": [
					{ "name": "SourceChainSelector", "type": "u64" },
					{ "name": "OnRampAddress", "type": "publicKey" },
					{ "name": "MinSeqNr", "type": "u64" },
					{ "name": "MaxSeqNr", "type": "u64" },
					{ "name": "MerkleRoot", "type": { "array": ["u8", 32] } }
				]
			}
		},
		{
			"name": "TokenPriceUpdate",
			"type": {
				"kind": "struct",
				"fields": [
					{ "name": "SourceToken", "type": "publicKey" },
					{ "name": "UsdPerToken", "type": "i128" }	
				]
			}
		},
		{
			"name": "GasPriceUpdate",
			"type": {
				"kind": "struct",
				"fields": [
					{ "name": "DestChainSelector", "type": "u64" },
					{ "name": "UsdPerUnitGas", "type": "i128" }	
				]
			}
		},
		{
			"name": "PriceUpdates",
			"type": {
				"kind": "struct",
				"fields": [
					{ "name": "TokenPriceUpdates", "type": { "vec": { "defined": "TokenPriceUpdate" } } },
					{ "name": "GasPriceUpdates", "type": { "vec": { "defined": "GasPriceUpdate" } } }	
				]
			}
		}
	],
	"events": [
		{
			"name": "CommitReportAcceptedEvent",
			"fields": [
				{"name": "BlessedMerkleRoots", "type": {"vec": {"defined": "MerkleRoot"}}},
				{"name": "UnblessedMerkleRoots", "type": {"vec": {"defined": "MerkleRoot"}}},
				{"name": "PriceUpdates", "type": {"defined": "PriceUpdates"}}
			]
		}
	]
}
`
