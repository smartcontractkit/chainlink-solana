package solana_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const (
	Namespace   = "NameSpace"
	NamedMethod = "NamedMethod1"
)

func TestSolanaChainReaderService_ServiceCtx(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)
	svc, err := solana.NewChainReaderService(logger.Test(t), new(mockedRPCClient), config.ChainReader{})

	require.NoError(t, err)
	require.NotNil(t, svc)

	require.Error(t, svc.Ready())
	require.Len(t, svc.HealthReport(), 1)
	require.Contains(t, svc.HealthReport(), solana.ServiceName)
	require.Error(t, svc.HealthReport()[solana.ServiceName])

	require.NoError(t, svc.Start(ctx))
	require.NoError(t, svc.Ready())
	require.Equal(t, map[string]error{solana.ServiceName: nil}, svc.HealthReport())

	require.Error(t, svc.Start(ctx))

	require.NoError(t, svc.Close())
	require.Error(t, svc.Ready())
	require.Error(t, svc.Close())
}

func TestSolanaChainReaderService_GetLatestValue(t *testing.T) {
	t.Parallel()

	ctx := tests.Context(t)

	// encode values from unmodified test struct to be read and decoded
	expected := codec.DefaultTestStruct

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		testCodec, conf := newTestConfAndCodec(t)
		encoded, err := testCodec.Encode(ctx, expected, codec.TestStructWithNestedStruct)

		require.NoError(t, err)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		client.On("ReadAll", mock.Anything, mock.Anything).Return(encoded, nil)

		var result modifiedStructWithNestedStruct

		require.NoError(t, svc.GetLatestValue(ctx, Namespace, NamedMethod, nil, &result))
		assert.Equal(t, expected.InnerStruct, result.InnerStruct)
		assert.Equal(t, expected.Value, result.V)
		assert.Equal(t, expected.TimeVal, result.TimeVal)
		assert.Equal(t, expected.DurationVal, result.DurationVal)
	})

	t.Run("Error Returned From Account Reader", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		expectedErr := fmt.Errorf("expected error")
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		client.On("ReadAll", mock.Anything, mock.Anything).Return(nil, expectedErr)

		var result modifiedStructWithNestedStruct

		assert.ErrorIs(t, svc.GetLatestValue(ctx, Namespace, NamedMethod, nil, &result), expectedErr)
	})

	t.Run("Method Not Found", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		var result modifiedStructWithNestedStruct

		assert.NotNil(t, svc.GetLatestValue(ctx, Namespace, "Unknown", nil, &result))
	})

	t.Run("Namespace Not Found", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		var result modifiedStructWithNestedStruct

		assert.NotNil(t, svc.GetLatestValue(ctx, "Unknown", "Unknown", nil, &result))
	})

	t.Run("Bind Success", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		pk := ag_solana.NewWallet().PublicKey()
		err = svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, NamedMethod, 0),
			},
		})

		assert.NoError(t, err)
	})

	t.Run("Bind Errors", func(t *testing.T) {
		t.Parallel()

		_, conf := newTestConfAndCodec(t)

		client := new(mockedRPCClient)
		svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

		require.NoError(t, err)
		require.NotNil(t, svc)

		pk := ag_solana.NewWallet().PublicKey()

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    "incorrect format",
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", "Unknown", "Unknown", 0),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, "Unknown", 0),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, NamedMethod, 1),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: pk.String(),
				Name:    fmt.Sprintf("%s.%s.o", Namespace, NamedMethod),
			},
		}))

		require.NotNil(t, svc.Bind(ctx, []types.BoundContract{
			{
				Address: "invalid",
				Name:    fmt.Sprintf("%s.%s.%d", Namespace, NamedMethod, 0),
			},
		}))
	})
}

func newTestConfAndCodec(t *testing.T) (encodings.CodecFromTypeCodec, config.ChainReader) {
	t.Helper()

	rawIDL, _, testCodec := codec.NewTestIDLAndCodec(t)
	conf := config.ChainReader{
		Namespaces: map[string]config.ChainReaderMethods{
			Namespace: {
				Methods: map[string]config.ChainDataReader{
					NamedMethod: {
						AnchorIDL: rawIDL,
						Procedures: []config.ChainReaderProcedure{
							{
								IDLAccount: codec.TestStructWithNestedStruct,
								Type:       config.ProcedureTypeAnchor,
								OutputModifications: codeccommon.ModifiersConfig{
									&codeccommon.RenameModifierConfig{Fields: map[string]string{"Value": "V"}},
								},
							},
						},
					},
				},
			},
		},
	}

	return testCodec, conf
}

type modifiedStructWithNestedStruct struct {
	V                uint8
	InnerStruct      codec.ObjectRef1
	BasicNestedArray [][]uint32
	Option           *string
	DefinedArray     []codec.ObjectRef2
	BasicVector      []string
	TimeVal          int64
	DurationVal      time.Duration
	PublicKey        ag_solana.PublicKey
	EnumVal          uint8
}

type mockedRPCClient struct {
	mock.Mock
}

func (_m *mockedRPCClient) ReadAll(ctx context.Context, pk ag_solana.PublicKey) ([]byte, error) {
	ret := _m.Called(ctx, pk)

	var r0 []byte

	if val, ok := ret.Get(0).([]byte); ok {
		r0 = val
	}

	return r0, ret.Error(1)
}
