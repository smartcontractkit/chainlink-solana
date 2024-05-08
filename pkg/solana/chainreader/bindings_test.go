package chainreader

import (
	"context"
	"reflect"
	"testing"

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

	t.Run("multiple bindings return merged struct", func(t *testing.T) {
		t.Parallel()

		bindingA := new(mockBinding)
		bindingB := new(mockBinding)
		bindings := namespaceBindings{}

		bindings.AddReadBinding("A", "B", bindingA)
		bindings.AddReadBinding("A", "B", bindingB)

		bindingA.On("CreateType", mock.Anything).Return(struct{ A string }{A: "test"}, nil)
		bindingB.On("CreateType", mock.Anything).Return(struct{ B int }{B: 8}, nil)

		result, err := bindings.CreateType("A", "B", true)

		expected := reflect.New(reflect.StructOf([]reflect.StructField{
			{Name: "A", Type: reflect.TypeOf("")},
			{Name: "B", Type: reflect.TypeOf(0)},
		}))

		require.NoError(t, err)
		assert.Equal(t, expected.Type(), reflect.TypeOf(result))
	})

	t.Run("multiple bindings fails when not a struct", func(t *testing.T) {
		t.Parallel()

		bindingA := new(mockBinding)
		bindingB := new(mockBinding)
		bindings := namespaceBindings{}

		bindings.AddReadBinding("A", "B", bindingA)
		bindings.AddReadBinding("A", "B", bindingB)

		bindingA.On("CreateType", mock.Anything).Return(8, nil)
		bindingB.On("CreateType", mock.Anything).Return(struct{ A string }{A: "test"}, nil)

		_, err := bindings.CreateType("A", "B", true)

		require.ErrorIs(t, err, types.ErrInvalidType)
	})

	t.Run("multiple bindings errors when fields overlap", func(t *testing.T) {
		t.Parallel()

		bindingA := new(mockBinding)
		bindingB := new(mockBinding)
		bindings := namespaceBindings{}

		bindings.AddReadBinding("A", "B", bindingA)
		bindings.AddReadBinding("A", "B", bindingB)

		type A struct {
			A string
			B int
		}

		type B struct {
			A int
		}

		bindingA.On("CreateType", mock.Anything).Return(A{A: ""}, nil)
		bindingB.On("CreateType", mock.Anything).Return(B{A: 8}, nil)

		_, err := bindings.CreateType("A", "B", true)

		require.ErrorIs(t, err, types.ErrInvalidConfig)
	})
}

type mockBinding struct {
	mock.Mock
}

func (_m *mockBinding) PreLoad(context.Context, *loadedResult) {}

func (_m *mockBinding) GetLatestValue(ctx context.Context, params, returnVal any, _ *loadedResult) error {
	return nil
}

func (_m *mockBinding) Bind(types.BoundContract) error {
	return nil
}

func (_m *mockBinding) CreateType(b bool) (any, error) {
	ret := _m.Called(b)

	return ret.Get(0), ret.Error(1)
}
