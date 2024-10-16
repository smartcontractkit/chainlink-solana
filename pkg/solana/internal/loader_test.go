package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testLoader struct {
	Loader[any]
	callCount int
}

func (t *testLoader) load() (any, error) {
	t.callCount++
	return nil, nil
}

func newTestLoader(lazyLoad bool) *testLoader {
	loader := testLoader{}
	loader.Loader = NewLoader[any](lazyLoad, loader.load)
	return &loader
}

func TestLoader(t *testing.T) {
	t.Run("direct loading", func(t *testing.T) {
		loader := newTestLoader(false)
		_, _ = loader.Get()
		_, _ = loader.Get()
		_, _ = loader.Get()
		require.Equal(t, 3, loader.callCount)
	})

	t.Run("lazy loading", func(t *testing.T) {
		loader := newTestLoader(true)
		_, _ = loader.Get()
		_, _ = loader.Get()
		require.Equal(t, 1, loader.callCount)

		// Calls load function again after Reset()
		loader.Reset()
		_, _ = loader.Get()
		_, _ = loader.Get()
		require.Equal(t, 2, loader.callCount)
	})
}
