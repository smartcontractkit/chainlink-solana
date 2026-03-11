package chainreader

import (
	"context"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestBindings_CreateType(t *testing.T) {
	t.Parallel()

	t.Run("single binding returns type", func(t *testing.T) {
		t.Parallel()

		expected := 8
		bdRegistry := newBindingsRegistry()
		binding := new(mockBinding)

		bdRegistry.AddReader("A", "B", binding)
		binding.On("CreateType", mock.Anything).Return(expected, nil)

		returned, err := bdRegistry.CreateType("A", "B", true)

		require.NoError(t, err)
		assert.Equal(t, expected, returned)
	})

	t.Run("returns error when binding does not exist", func(t *testing.T) {
		t.Parallel()

		bdRegistry := newBindingsRegistry()
		_, err := bdRegistry.CreateType("A", "B", true)

		require.ErrorIs(t, err, types.ErrInvalidConfig)
	})
}

type mockBinding struct {
	mock.Mock
}

func (_m *mockBinding) Bind(ctx context.Context, address solana.PublicKey) error {
	ret := _m.Called(ctx, address)
	return ret.Error(0)
}

func (_m *mockBinding) Unbind(_ context.Context) error { return nil }

func (_m *mockBinding) SetCodec(_ types.RemoteCodec) {}

func (_m *mockBinding) Register(_ context.Context) error { return nil }

func (_m *mockBinding) Unregister(_ context.Context) error { return nil }

func (_m *mockBinding) GetAddress(_ context.Context, _ any) (solana.PublicKey, error) {
	return solana.PublicKey{}, nil
}

func (_m *mockBinding) GetGenericName() string {
	return ""
}

func (_m *mockBinding) GetReadDefinition() config.ReadDefinition {
	return config.ReadDefinition{}
}

func (_m *mockBinding) GetIDLInfo() (idl codecv1.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codecv1.IdlTypeDef) {
	return codecv1.IDL{}, codecv1.IdlTypeDef{}, codecv1.IdlTypeDef{}
}

func (_m *mockBinding) GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig {
	return &commoncodec.HardCodeModifierConfig{}
}

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

func Test_namespaceBinding_BindReaders(t *testing.T) {
	type fields struct {
		name    string
		readers map[string]readBinding
		bound   map[solana.PublicKey]bool
	}
	tts := []struct {
		name    string
		fields  fields
		address solana.PublicKey
		wantErr bool
	}{
		{
			name: "no readers",
			fields: fields{
				name:    "testNamespace",
				readers: make(map[string]readBinding),
				bound:   make(map[solana.PublicKey]bool),
			},
			address: solana.PublicKey{},
			wantErr: false,
		},
		{
			name: "single reader binds successfully",
			fields: fields{
				name: "testNamespace",
				readers: map[string]readBinding{
					"reader1": func() *mockBinding {
						m := &mockBinding{}
						m.On("Bind", mock.Anything, mock.Anything).Return(nil)
						return m
					}(),
				},
				bound: make(map[solana.PublicKey]bool),
			},
			address: solana.PublicKey{},
			wantErr: false,
		},
		{
			name: "multiple readers bind successfully",
			fields: fields{
				name: "testNamespace",
				readers: map[string]readBinding{
					"reader1": func() *mockBinding {
						m := &mockBinding{}
						m.On("Bind", mock.Anything, mock.Anything).Return(nil)
						return m
					}(),
					"reader2": func() *mockBinding {
						m := &mockBinding{}
						m.On("Bind", mock.Anything, mock.Anything).Return(nil)
						return m
					}(),
				},
				bound: make(map[solana.PublicKey]bool),
			},
			address: solana.PublicKey{},
			wantErr: false,
		},
		{
			name: "reader bind returns error",
			fields: fields{
				name: "testNamespace",
				readers: map[string]readBinding{
					"reader1": func() *mockBinding {
						m := &mockBinding{}
						// Returns nil to show the first reader binds successfully
						m.On("Bind", mock.Anything, mock.Anything).Return(nil)
						return m
					}(),
					"reader2": func() *mockBinding {
						m := &mockBinding{}
						// Returns an error to simulate a bind failure
						m.On("Bind", mock.Anything, mock.Anything).Return(fmt.Errorf("bind error"))
						return m
					}(),
				},
				bound: make(map[solana.PublicKey]bool),
			},
			address: solana.PublicKey{},
			wantErr: true,
		},
	}
	for _, tt := range tts {
		t.Run(tt.name, func(t *testing.T) {
			b := &namespaceBinding{
				name:    tt.fields.name,
				readers: tt.fields.readers,
				bound:   tt.fields.bound,
			}
			err := b.BindReaders(t.Context(), tt.address)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
