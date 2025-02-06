package utils

import (
	"context"
	"sync"
)

type Loader[T any] interface {
	Get(context.Context) (T, error)
	Reset()
}

var _ Loader[any] = (*onceLoader[any])(nil)

type onceLoader[T any] struct {
	getClient func(ctx context.Context) (T, error)
}

func (c *onceLoader[T]) Get(ctx context.Context) (T, error) {
	return c.getClient(ctx)
}

func (c *onceLoader[T]) Reset() { /* do nothing */ }

func NewOnceLoader[T any](getClient func(ctx context.Context) (T, error)) *onceLoader[T] {
	return &onceLoader[T]{
		getClient: getClient,
	}
}

var _ Loader[any] = (*loader[any])(nil)

type loader[T any] struct {
	getClient func(ctx context.Context) (T, error)
	state     T
	ok        bool
	lock      sync.Mutex
}

func (c *loader[T]) Get(ctx context.Context) (out T, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.ok {
		return c.state, nil
	}
	c.state, err = c.getClient(ctx)
	c.ok = err == nil
	return c.state, err
}

func (c *loader[T]) Reset() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ok = false
}

func NewLoader[T any](getClient func(ctx context.Context) (T, error)) *loader[T] {
	return &loader[T]{
		getClient: getClient,
	}
}

var _ Loader[any] = (*staticLoader[any])(nil)

type staticLoader[T any] struct {
	val T
}

func (s *staticLoader[T]) Get(ctx context.Context) (T, error) {
	return s.val, nil
}

func (s *staticLoader[T]) Reset() {}

func NewStaticLoader[T any](v T) Loader[T] {
	return &staticLoader[T]{v}
}
