package chainreader

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestBindings_CreateType(t *testing.T) {
	t.Parallel()

	t.Run("single binding returns type", func(t *testing.T) {
		t.Parallel()

		expected := 8
		bdRegistry := bindingsRegistry{namespaceBindings: make(map[string]readNameBindings)}
		binding := new(mockBinding)
		bdRegistry.AddReadBinding("A", "B", binding)

		binding.On("CreateType", mock.Anything).Return(expected, nil)

		returned, err := bdRegistry.CreateType("A", "B", true)

		require.NoError(t, err)
		assert.Equal(t, expected, returned)
	})

	t.Run("returns error when binding does not exist", func(t *testing.T) {
		t.Parallel()

		bdRegistry := bindingsRegistry{namespaceBindings: make(map[string]readNameBindings)}

		_, err := bdRegistry.CreateType("A", "B", true)

		require.ErrorIs(t, err, types.ErrInvalidConfig)
	})
}

type mockBinding struct {
	mock.Mock
}

func (_m *mockBinding) GetAddress(_ context.Context, _ any) (solana.PublicKey, error) {
	return solana.PublicKey{}, nil
}

func (_m *mockBinding) GetGenericName() string {
	return ""
}

func (_m *mockBinding) GetReadDefinition() config.ReadDefinition {
	return config.ReadDefinition{}
}

func (_m *mockBinding) GetIDLInfo() (idl codec.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codec.IdlTypeDef) {
	return codec.IDL{}, codec.IdlTypeDef{}, codec.IdlTypeDef{}
}

func (_m *mockBinding) GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig {
	return &commoncodec.HardCodeModifierConfig{}
}

func (_m *mockBinding) SetAddress(_ solana.PublicKey) {}

func (_m *mockBinding) SetCodec(_ types.RemoteCodec) {}

func (_m *mockBinding) SetModifier(a commoncodec.Modifier) {
	_m.Called(a)
}

func (_m *mockBinding) CreateType(b bool) (any, error) {
	ret := _m.Called(b)

	return ret.Get(0), ret.Error(1)
}

func (_m *mockBinding) Decode(_ context.Context, _ []byte, _ any) error {
	return nil
}

func (_m *mockBinding) QueryKey(
	a context.Context,
	b query.KeyFilter,
	c query.LimitAndSort,
	d any,
) ([]types.Sequence, error) {
	ret := _m.Called(a, b, c, d)

	return ret.Get(0).([]types.Sequence), ret.Error(1)
}
