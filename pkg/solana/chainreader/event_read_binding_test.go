package chainreader

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainreader/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestGetLatestValue(t *testing.T) {
	t.Parallel()

	type offChainParams struct {
		A *int32
		B string
		C uint64
		D []byte
	}

	type offChainType struct{}

	subkeys := newIndexedSubkeys()
	subkeys.addForIndex("A", "W", 0)
	subkeys.addForIndex("B", "X", 1)
	subkeys.addForIndex("C", "Y", 2)
	subkeys.addForIndex("D", "Z", 3)

	readDef := config.ReadDefinition{}
	pollerConf := config.PollingFilter{}

	address := solana.NewWallet().PublicKey()
	parsed := newTestCodec(t)

	testCodec, err := parsed.ToCodec()

	require.NoError(t, err)

	t.Run("no params passes a limited filter to event source", func(t *testing.T) {
		t.Parallel()

		lpSource := new(mocks.EventsReader)
		reader := newEventReadBinding(namespace, genericName, subkeys, lpSource, readDef, pollerConf)
		ctx := tests.Context(t)

		require.NoError(t, reader.Bind(ctx, address))
		reader.SetCodec(testCodec)
		reader.SetModifier(parsed.Modifiers)

		lpSource.EXPECT().FilteredLogs(mock.Anything, mock.MatchedBy(expressionMatcher(t, 2)), mock.Anything, mock.Anything).Return(nil, nil)

		var offChainValue offChainType

		require.ErrorIs(t, reader.GetLatestValue(ctx, nil, &offChainValue), types.ErrNotFound)
	})

	t.Run("limited params set are extracted", func(t *testing.T) {
		t.Parallel()

		lpSource := new(mocks.EventsReader)
		reader := newEventReadBinding(namespace, genericName, subkeys, lpSource, readDef, pollerConf)
		ctx := tests.Context(t)

		require.NoError(t, reader.Bind(ctx, address))
		reader.SetCodec(testCodec)
		reader.SetModifier(parsed.Modifiers)

		lpSource.EXPECT().FilteredLogs(mock.Anything, mock.MatchedBy(expressionMatcher(t, 3)), mock.Anything, mock.Anything).Return(nil, nil)

		var offChainValue offChainType

		require.ErrorIs(t, reader.GetLatestValue(ctx, map[string]any{"A": int32(4)}, &offChainValue), types.ErrNotFound)
	})

	t.Run("full params list is passed to eent source", func(t *testing.T) {
		t.Parallel()

		lpSource := new(mocks.EventsReader)
		reader := newEventReadBinding(namespace, genericName, subkeys, lpSource, readDef, pollerConf)
		ctx := tests.Context(t)

		require.NoError(t, reader.Bind(ctx, address))
		reader.SetCodec(testCodec)
		reader.SetModifier(parsed.Modifiers)

		lpSource.EXPECT().FilteredLogs(mock.Anything, mock.MatchedBy(expressionMatcher(t, 6)), mock.Anything, mock.Anything).Return(nil, nil)

		var (
			intVal = int32(42)
		)

		params := &offChainParams{
			A: &intVal,
			B: "test",
			C: uint64(42),
			D: []byte("test"),
		}

		var offChainValue offChainType

		require.ErrorIs(t, reader.GetLatestValue(ctx, params, &offChainValue), types.ErrNotFound)
	})
}

func expressionMatcher(t *testing.T, count int) func([]query.Expression) bool {
	t.Helper()

	return func(expressions []query.Expression) bool {
		t.Helper()

		var c int

		for _, exp := range expressions {
			if exp.Primitive == nil {
				c += len(exp.BoolExpression.Expressions)

				continue
			}

			c++
		}

		return c == count
	}
}

func newTestCodec(t *testing.T) *codec.ParsedTypes {
	t.Helper()

	rawIDL := fmt.Sprintf(basicEventIDL, testParamType)

	var IDL codec.IDL
	require.NoError(t, json.Unmarshal([]byte(rawIDL), &IDL))

	idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeEventDef, "EventType", IDL)

	require.NoError(t, err)

	entry, err := codec.CreateCodecEntry(idlDef, "GenericName", IDL, commoncodec.NewPathTraverseRenamer(map[string]string{"W": "A", "X": "B", "Y": "C", "Z": "D"}, true))

	require.NoError(t, err)

	return &codec.ParsedTypes{
		EncoderDefs: map[string]codec.Entry{codec.WrapItemType(true, namespace, genericName): entry},
		DecoderDefs: map[string]codec.Entry{},
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
			{"name": "w", "type": {"option": "i32"}},
			{"name": "x", "type": "string"},
			{"name": "y", "type": "u64"},
			{"name": "z", "type": "bytes"}
		]
	}`
)
