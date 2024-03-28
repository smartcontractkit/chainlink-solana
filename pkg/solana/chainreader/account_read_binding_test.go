package chainreader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func TestPreload(t *testing.T) {
	t.Parallel()

	testCodec := makeTestCodec(t)

	t.Run("get latest value waits for preload", func(t *testing.T) {
		t.Parallel()

		reader := new(mockReader)
		binding := newAccountReadBinding(testCodecKey, testCodec, reader, nil)

		expected := testStruct{A: true, B: 42}
		bts, err := testCodec.Encode(context.Background(), expected, testCodecKey)

		require.NoError(t, err)

		reader.On("ReadAll", mock.Anything, mock.Anything, mock.Anything).Return(bts, nil).After(time.Second)

		ctx := context.Background()
		start := time.Now()
		loaded := &loadedResult{
			value: make(chan []byte, 1),
			err:   make(chan error, 1),
		}

		binding.PreLoad(ctx, loaded)

		var result testStruct

		err = binding.GetLatestValue(ctx, nil, &result, loaded)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, elapsed, time.Second)
		assert.Less(t, elapsed, 1100*time.Millisecond)
		assert.Equal(t, expected, result)
	})

	t.Run("cancelled context exits preload and returns error on get latest value", func(t *testing.T) {
		t.Parallel()

		reader := new(mockReader)
		binding := newAccountReadBinding(testCodecKey, testCodec, reader, nil)

		ctx, cancel := context.WithCancelCause(context.Background())

		// make the readall pause until after the context is cancelled
		reader.On("ReadAll", mock.Anything, mock.Anything, mock.Anything).
			Return([]byte{}, nil).
			After(600 * time.Millisecond)

		expectedErr := errors.New("test error")
		go func() {
			time.Sleep(500 * time.Millisecond)
			cancel(expectedErr)
		}()

		loaded := &loadedResult{
			value: make(chan []byte, 1),
			err:   make(chan error, 1),
		}
		start := time.Now()
		binding.PreLoad(ctx, loaded)

		var result testStruct
		err := binding.GetLatestValue(ctx, nil, &result, loaded)
		elapsed := time.Since(start)

		assert.ErrorIs(t, err, ctx.Err())
		assert.ErrorIs(t, context.Cause(ctx), expectedErr)
		assert.GreaterOrEqual(t, elapsed, 600*time.Millisecond)
		assert.Less(t, elapsed, 700*time.Millisecond)
	})

	t.Run("error from preload is returned in get latest value", func(t *testing.T) {
		t.Parallel()

		reader := new(mockReader)
		binding := newAccountReadBinding(testCodecKey, testCodec, reader, nil)
		ctx := context.Background()
		expectedErr := errors.New("test error")

		reader.On("ReadAll", mock.Anything, mock.Anything, mock.Anything).
			Return([]byte{}, expectedErr)

		loaded := &loadedResult{
			value: make(chan []byte, 1),
			err:   make(chan error, 1),
		}
		binding.PreLoad(ctx, loaded)

		var result testStruct
		err := binding.GetLatestValue(ctx, nil, &result, loaded)

		assert.ErrorIs(t, err, expectedErr)
	})
}

type mockReader struct {
	mock.Mock
}

func (_m *mockReader) ReadAll(ctx context.Context, pk solana.PublicKey, opts *rpc.GetAccountInfoOpts) ([]byte, error) {
	ret := _m.Called(ctx, pk)

	var r0 []byte
	if val, ok := ret.Get(0).([]byte); ok {
		r0 = val
	}

	var r1 error
	if fn, ok := ret.Get(1).(func() error); ok {
		r1 = fn()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type testStruct struct {
	A bool
	B int64
}

const testCodecKey = "TEST"

func makeTestCodec(t *testing.T) types.RemoteCodec {
	t.Helper()

	builder := binary.LittleEndian()

	structCodec, err := encodings.NewStructCodec([]encodings.NamedTypeCodec{
		{Name: "A", Codec: builder.Bool()},
		{Name: "B", Codec: builder.Int64()},
	})

	require.NoError(t, err)

	return encodings.CodecFromTypeCodec(map[string]encodings.TypeCodec{testCodecKey: structCodec})
}
