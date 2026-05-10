package reactive

// DisposeFunc stops a reactive effect from observing future signal changes.
type DisposeFunc func()

// Dispose calls d when d is non-nil. It lets callers store the return value of
// Effect and dispose it with method syntax.
func (d DisposeFunc) Dispose() {
	if d != nil {
		d()
	}
}

type source interface {
	unsubscribe(*effect)
}

type effect struct {
	fn       func()
	deps     map[source]struct{}
	disposed bool
	running  bool
}

var current *effect

// Signal is a mutable reactive value for browser/UI event-loop state. Reading
// a signal inside Effect subscribes that effect to future Set calls, so only
// computations that actually read the changed signal run again.
type Signal[T any] struct {
	value T
	equal func(T, T) bool
	subs  map[*effect]struct{}
}

// NewSignal creates a signal with an initial comparable value. Set is a no-op
// when the new value compares equal to the old value.
func NewSignal[T comparable](initial T) *Signal[T] {
	return NewSignalFunc(initial, func(a, b T) bool { return a == b })
}

// NewSignalFunc creates a signal with custom equality. Use this for values
// that are not comparable, such as slices or maps. If equal is nil, every Set
// is treated as a change.
func NewSignalFunc[T any](initial T, equal func(T, T) bool) *Signal[T] {
	return &Signal[T]{value: initial, equal: equal}
}

// Get returns the current value and, when called inside Effect, records a
// dependency on this signal.
func (s *Signal[T]) Get() T {
	if current != nil && !current.disposed {
		if s.subs == nil {
			s.subs = map[*effect]struct{}{}
		}
		s.subs[current] = struct{}{}
		if current.deps == nil {
			current.deps = map[source]struct{}{}
		}
		current.deps[s] = struct{}{}
	}
	return s.value
}

// Peek returns the current value without subscribing the current effect.
func (s *Signal[T]) Peek() T { return s.value }

// Set stores a new value and re-runs only the effects that read this signal.
// Setting an equal value is a no-op.
func (s *Signal[T]) Set(next T) {
	if s.equal != nil && s.equal(s.value, next) {
		return
	}
	s.value = next
	subs := make([]*effect, 0, len(s.subs))
	for e := range s.subs {
		if !e.disposed {
			subs = append(subs, e)
		}
	}
	for _, e := range subs {
		e.run()
	}
}

// Update applies fn to the current value and stores the result.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.Peek())) }

func (s *Signal[T]) unsubscribe(e *effect) { delete(s.subs, e) }

// Effect runs fn immediately and then re-runs it whenever any signal read by fn
// changes. Dependencies are tracked dynamically on every run, so conditional
// reads automatically subscribe and unsubscribe as branches change.
func Effect(fn func()) DisposeFunc {
	e := &effect{fn: fn, deps: map[source]struct{}{}}
	e.run()
	return e.dispose
}

func (e *effect) run() {
	if e.disposed || e.running {
		return
	}
	e.running = true
	for dep := range e.deps {
		dep.unsubscribe(e)
	}
	e.deps = map[source]struct{}{}
	prev := current
	current = e
	defer func() {
		current = prev
		e.running = false
	}()

	e.fn()
}

func (e *effect) dispose() {
	if e.disposed {
		return
	}
	e.disposed = true
	for dep := range e.deps {
		dep.unsubscribe(e)
	}
	e.deps = nil
}
