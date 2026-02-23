package logpoller

// ---
// A very simple stack implementation.
type stack[T any] struct {
	items []T
}

func newStack[T any]() stack[T] {
	return stack[T]{
		items: []T{},
	}
}

func (s *stack[T]) Depth() int {
	return len(s.items)
}

func (s *stack[T]) Push(item T) {
	s.items = append(s.items, item)
}

func (s *stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	item := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return item, true
}

func (s *stack[T]) Peek() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

// callers must validate that len > 0 before calling, otherwise this will panic
func (s *stack[T]) PeekUnchecked() T {
	return s.items[len(s.items)-1]
}

// ---
// A very simple append-only list implementation.
type appendOnly[T any] struct {
	items []T
}

func newAppendOnly[T any]() appendOnly[T] {
	return appendOnly[T]{
		items: []T{},
	}
}

func (a *appendOnly[T]) Append(item T) {
	a.items = append(a.items, item)
}

func (a *appendOnly[T]) At(i int) (T, bool) {
	if i < 0 || i >= a.Len() {
		var zero T
		return zero, false
	}
	return a.items[i], true
}

func (a *appendOnly[T]) Len() int {
	return len(a.items)
}

// callers must validate that len > 0 before calling, otherwise this will panic
func (a *appendOnly[T]) PeekUnchecked() *T {
	return &a.items[len(a.items)-1]
}
