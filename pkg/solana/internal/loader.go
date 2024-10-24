package internal

type Loader[T any] interface {
	Get() (T, error)
	Reset()
}

var _ Loader[any] = (*loader[any])(nil)

type loader[T any] struct {
	getClient func() (T, error)
}

func (c *loader[T]) Get() (T, error) {
	return c.getClient()
}

func (c *loader[T]) Reset() { /* do nothing */ }

func NewLoader[T any](getClient func() (T, error)) *loader[T] {
	return &loader[T]{
		getClient: getClient,
	}
}
