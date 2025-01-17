package chainreader

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func TestBindings_CreateType(t *testing.T) {
	t.Parallel()

	t.Run("single binding returns type", func(t *testing.T) {
		t.Parallel()

		expected := 8
		binding := new(mockBinding)
		bindings := namespaceBindings{}
		bindings.AddReadBinding("A", "B", binding)

		binding.On("CreateType", mock.Anything).Return(expected, nil)

		returned, err := bindings.CreateType("A", "B", true)

		require.NoError(t, err)
		assert.Equal(t, expected, returned)
	})

	t.Run("returns error when binding does not exist", func(t *testing.T) {
		t.Parallel()

		bindings := namespaceBindings{}

		_, err := bindings.CreateType("A", "B", true)

		require.ErrorIs(t, err, types.ErrInvalidConfig)
	})
}

type mockBinding struct {
	mock.Mock
}

func (_m *mockBinding) SetCodec(_ types.RemoteCodec) {}

func (_m *mockBinding) SetAddress(_ solana.PublicKey) {}

func (_m *mockBinding) GetAddress(_ context.Context, _ any) (solana.PublicKey, error) {
	return solana.PublicKey{}, nil
}

func (_m *mockBinding) CreateType(b bool) (any, error) {
	ret := _m.Called(b)

	return ret.Get(0), ret.Error(1)
}

func (_m *mockBinding) Decode(_ context.Context, _ []byte, _ any) error {
	return nil
}
