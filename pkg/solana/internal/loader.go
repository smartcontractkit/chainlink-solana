package internal

import (
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
)

type Loader[T any] interface {
	Get() (T, error)
	Reset()
}

var _ Loader[any] = (*loader[any])(nil)

type loader[T any] struct {
	getClient  func() (T, error)
	lazyLoader *utils.LazyLoad[T]
}

func (c *loader[T]) Get() (T, error) {
	if c.lazyLoader != nil {
		return c.lazyLoader.Get()
	}
	return c.getClient()
}

func (c *loader[T]) Reset() {
	if c.lazyLoader != nil {
		c.lazyLoader.Reset()
	}
}

func NewLoader[T any](lazyLoad bool, getClient func() (T, error)) *loader[T] {
	var lazyLoader *utils.LazyLoad[T]
	if lazyLoad {
		lazyLoader = utils.NewLazyLoad(getClient)
	}
	return &loader[T]{
		lazyLoader: lazyLoader,
		getClient:  getClient,
	}
}
