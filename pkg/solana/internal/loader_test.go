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

func newTestLoader() *testLoader {
	loader := testLoader{}
	loader.Loader = NewLoader[any](loader.load)
	return &loader
}

func TestLoader(t *testing.T) {
	t.Run("direct loading", func(t *testing.T) {
		loader := newTestLoader()
		_, _ = loader.Get()
		_, _ = loader.Get()
		_, _ = loader.Get()
		require.Equal(t, 3, loader.callCount)
	})
}
