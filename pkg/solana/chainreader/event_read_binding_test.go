package chainreader

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainreader/mocks"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestBind(t *testing.T) {
	address1 := solana.PublicKey{1, 2, 3}
	address2 := solana.PublicKey{6, 5, 4}

	subkeys := newIndexedSubkeys()
	subkeys.addForIndex("A", "W", 0)
	subkeys.addForIndex("B", "X", 1)
	subkeys.addForIndex("C", "Y", 2)
	subkeys.addForIndex("D", "Z", 3)

	subkeys2 := newIndexedSubkeys()
	subkeys2.addForIndex("A", "W", 0)
	subkeys2.addForIndex("B", "X", 1)

	readDef := config.ReadDefinition{}
	pollerConf := config.PollingFilter{}

	t.Run("Bind twice is noop", func(t *testing.T) {
		t.Parallel()

		lpSource := new(mocks.EventsReader)

		lpSource.EXPECT().HasFilter(mock.Anything, mock.Anything).Return(false)
		lpSource.EXPECT().RegisterFilter(mock.Anything, mock.Anything).Return(nil).Twice() // 1 per address 1 and 1 per address2

		reader := newEventReadBinding(namespace, genericName, subkeys, lpSource, readDef, pollerConf)
		ctx := t.Context()

		require.NoError(t, reader.Register(ctx))
		require.NoError(t, reader.Bind(ctx, address1))
		require.NoError(t, reader.Bind(ctx, address1))
		require.NoError(t, reader.Bind(ctx, address1))

		require.NoError(t, reader.Bind(ctx, address2))
		lpSource.AssertExpectations(t)
	})

	t.Run("Bind derives name based on filter", func(t *testing.T) {
		t.Parallel()

		lpSource := new(mocks.EventsReader)

		lpSource.EXPECT().HasFilter(mock.Anything, mock.Anything).Return(false)
		lpSource.EXPECT().RegisterFilter(mock.Anything, mock.Anything).Return(nil).Twice() // 1 per address 1 and 1 per address2

		reader := newEventReadBinding(namespace, genericName, subkeys, lpSource, readDef, pollerConf)
		reader2 := newEventReadBinding(namespace, genericName, subkeys2, lpSource, readDef, pollerConf)
		name0 := reader.deriveName()
		assert.Equal(t, "TestNamespace.GenericName.64534094550029c2338585738a654173ff263d471b8728e30f147ce68451cd0b", name0)
		name := reader2.deriveName()
		assert.Equal(t, "TestNamespace.GenericName.9a5cc2ed54afdbd1136f222be651c4ad12afbc95f6438e7dddc7c92e4532156f", name)
		require.NotEqual(t, name0, name)
		ctx := t.Context()

		require.NoError(t, reader.Register(ctx))
		require.NoError(t, reader.Bind(ctx, address1))

		// name should have changed
		name1 := reader.deriveName()
		assert.Equal(t, name1, reader.deriveName())
		assert.Equal(t, "TestNamespace.GenericName.aa5a667222c55daaa0c5872453847c6eda5b0e07abacd3333b25425ad7ebd0b9", name1)
		assert.NotEqual(t, name0, name1)

		require.NoError(t, reader.Bind(ctx, address1))

		// name shoudln't have changed (address the same)
		name2 := reader.deriveName()
		require.Equal(t, name1, name2)

		require.NoError(t, reader.Bind(ctx, address2))

		// name should have changed
		name3 := reader.deriveName()
		require.NotEqual(t, name2, name3)
		assert.Equal(t, "TestNamespace.GenericName.f73ac95d7b8ff5aa315e8c035ced1fce78b9e15d1c09c75c9a636eca6add63f4", name3)
		lpSource.AssertExpectations(t)
	})

	t.Run("Unbind works correctly", func(t *testing.T) {
		t.Parallel()

		lpSource := new(mocks.EventsReader)

		reader := newEventReadBinding(namespace, genericName, subkeys, lpSource, readDef, pollerConf)
		ctx := t.Context()

		require.NoError(t, reader.Register(ctx))

		var ret bool
		lpSource.EXPECT().HasFilter(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, str string) bool {
			return ret
		})

		lpSource.EXPECT().RegisterFilter(mock.Anything, mock.Anything).Return(nil).Once()
		require.NoError(t, reader.Bind(ctx, address1))

		ret = true
		lpSource.AssertExpectations(t)

		lpSource.EXPECT().UnregisterFilter(mock.Anything, mock.Anything).Return(nil).Once()
		require.NoError(t, reader.Unbind(ctx))

		lpSource.AssertExpectations(t)
	})
}

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
		ctx := t.Context()

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
		ctx := t.Context()

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
		ctx := t.Context()

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

func newTestCodec(t *testing.T) *solcommoncodec.ParsedTypes {
	t.Helper()

	rawIDL := fmt.Sprintf(basicEventIDL, testParamType)

	var IDL codecv1.IDL
	require.NoError(t, json.Unmarshal([]byte(rawIDL), &IDL))

	idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, "EventType", IDL)

	require.NoError(t, err)

	entry, err := codecv1.CreateCodecEntry(idlDef, "GenericName", IDL, commoncodec.NewPathTraverseRenamer(map[string]string{"W": "A", "X": "B", "Y": "C", "Z": "D"}, true))

	require.NoError(t, err)

	return &solcommoncodec.ParsedTypes{
		EncoderDefs: map[string]solcommoncodec.Entry{solcommoncodec.WrapItemType(true, namespace, genericName): entry},
		DecoderDefs: map[string]solcommoncodec.Entry{},
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
