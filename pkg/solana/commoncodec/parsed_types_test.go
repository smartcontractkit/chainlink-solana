package commoncodec_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/commoncodec"
)

func TestEncodeDecodeBigInt(t *testing.T) {
	t.Parallel()

	type offChain struct {
		A *big.Int
		B *big.Int
	}

	ctx := t.Context()
	types := newTestCodec(t)
	typedCodec, err := types.ToCodec()

	require.NoError(t, err)

	value := offChain{
		A: big.NewInt(42),
		B: big.NewInt(42),
	}

	bts, err := typedCodec.Encode(ctx, &value, solcommoncodec.WrapItemType(true, namespace, genericName))

	require.NoError(t, err)

	var output offChain

	require.NoError(t, typedCodec.Decode(ctx, bts, &output, solcommoncodec.WrapItemType(false, namespace, genericName)))
	require.Equal(t, value.A.String(), output.A.String())
	require.Equal(t, value.B.String(), output.B.String())
}

func newTestCodec(t *testing.T) *solcommoncodec.ParsedTypes {
	t.Helper()

	rawIDL := fmt.Sprintf(basicEventIDL, testParamType)

	var IDL codec.IDL
	require.NoError(t, json.Unmarshal([]byte(rawIDL), &IDL))

	idlDef, err := codec.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, "EventType", IDL)

	require.NoError(t, err)

	mods := commoncodec.MultiModifier{
		commoncodec.NewRenamer(map[string]string{"X": "A", "Y": "B"}),
	}

	entry, err := codec.CreateCodecEntry(idlDef, "GenericName", IDL, mods)

	require.NoError(t, err)

	return &solcommoncodec.ParsedTypes{
		EncoderDefs: map[string]solcommoncodec.Entry{solcommoncodec.WrapItemType(true, namespace, genericName): entry},
		DecoderDefs: map[string]solcommoncodec.Entry{solcommoncodec.WrapItemType(false, namespace, genericName): entry},
	}
}

const (
	namespace   = "TestNamespace"
	genericName = "GenericName"

	basicEventIDL = `{
		"version": "0.1.0",
		"name": "some_test_idl",
		"events": [%s]
	}`

	testParamType = `{
		"name": "EventType",
		"fields": [
			{"name": "x", "type": "i128"},
			{"name": "y", "type": "u128"}
		]
	}`
)
