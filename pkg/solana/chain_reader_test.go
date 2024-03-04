package solana_test

import (
	"context"
	"testing"

	ag_solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
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

// TODO: this test still doesn't pass for modifiers
func TestSolanaChainReaderService_GetLatestValue(t *testing.T) {
	// t.Skip()
	t.Parallel()

	rawIDL, _, testCodec := codec.NewTestIDLAndCodec(t)
	ctx := tests.Context(t)
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

	client := new(mockedRPCClient)

	svc, err := solana.NewChainReaderService(logger.Test(t), client, conf)

	require.NoError(t, err)
	require.NotNil(t, svc)

	// encode values from unmodified test struct to be read and decoded
	expected := codec.DefaultTestStruct
	encoded, err := testCodec.Encode(ctx, expected, codec.TestStructWithNestedStruct)

	require.NoError(t, err)

	client.On("GetAccountInfoBinaryData", mock.Anything, mock.Anything).Return(encoded, nil)

	var result modifiedStructWithNestedStruct

	require.NoError(t, svc.GetLatestValue(ctx, Namespace, NamedMethod, nil, &result))
	assert.Equal(t, expected.InnerStruct, result.InnerStruct)
	//assert.Equal(t, expected.Value, result.V)
}

type modifiedStructWithNestedStruct struct {
	V                uint8
	InnerStruct      codec.ObjectRef1
	BasicNestedArray [][]uint32
	Option           *string
	DefinedArray     []codec.ObjectRef2
}

type mockedRPCClient struct {
	mock.Mock
}

func (_m *mockedRPCClient) GetAccountInfoBinaryData(ctx context.Context, pk ag_solana.PublicKey) ([]byte, error) {
	ret := _m.Called(ctx, pk)

	var r0 []byte

	if val, ok := ret.Get(0).([]byte); ok {
		r0 = val
	}

	return r0, ret.Error(1)
}
