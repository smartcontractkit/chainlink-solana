package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type testLoader struct {
	Loader[any]
	callCount int
}

func (t *testLoader) load(context.Context) (any, error) {
	t.callCount++
	return nil, nil
}

func newTestLoader() *testLoader {
	loader := testLoader{}
	loader.Loader = NewOnceLoader[any](loader.load)
	return &loader
}

func TestLoader(t *testing.T) {
	t.Run("direct loading", func(t *testing.T) {
		ctx := tests.Context(t)
		loader := newTestLoader()
		_, _ = loader.Get(ctx)
		_, _ = loader.Get(ctx)
		_, _ = loader.Get(ctx)
		require.Equal(t, 3, loader.callCount)
	})
}
