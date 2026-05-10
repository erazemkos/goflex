package reactive

import (
	"reflect"
	"sync"
)

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
	unsubscribeLocked(*effect)
}

type effect struct {
	fn       func()
	deps     map[source]struct{}
	disposed bool
	running  bool
}

var runtime = struct {
	sync.Mutex
	current *effect
}{}

// Signal is a mutable reactive value for browser/UI event-loop state. Reading
// a signal inside Effect subscribes that effect to future Set calls, so only
// computations that actually read the changed signal run again.
type Signal[T any] struct {
	value T
	subs  map[*effect]struct{}
}

// NewSignal creates a signal with an initial value.
func NewSignal[T any](initial T) *Signal[T] { return &Signal[T]{value: initial} }

// Get returns the current value and, when called inside Effect, records a
// dependency on this signal.
func (s *Signal[T]) Get() T {
	runtime.Lock()
	defer runtime.Unlock()
	if runtime.current != nil && !runtime.current.disposed {
		if s.subs == nil {
			s.subs = map[*effect]struct{}{}
		}
		s.subs[runtime.current] = struct{}{}
		if runtime.current.deps == nil {
			runtime.current.deps = map[source]struct{}{}
		}
		runtime.current.deps[s] = struct{}{}
	}
	return s.value
}

// Peek returns the current value without subscribing the current effect.
func (s *Signal[T]) Peek() T {
	runtime.Lock()
	defer runtime.Unlock()
	return s.value
}

// Set stores a new value and re-runs only the effects that read this signal.
// Setting an equal value is a no-op.
func (s *Signal[T]) Set(next T) {
	runtime.Lock()
	if reflect.DeepEqual(s.value, next) {
		runtime.Unlock()
		return
	}
	s.value = next
	subs := make([]*effect, 0, len(s.subs))
	for e := range s.subs {
		if !e.disposed {
			subs = append(subs, e)
		}
	}
	runtime.Unlock()

	for _, e := range subs {
		e.run()
	}
}

// Update applies fn to the current value and stores the result.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.Peek())) }

func (s *Signal[T]) unsubscribeLocked(e *effect) { delete(s.subs, e) }

// Effect runs fn immediately and then re-runs it whenever any signal read by fn
// changes. Dependencies are tracked dynamically on every run, so conditional
// reads automatically subscribe and unsubscribe as branches change.
func Effect(fn func()) DisposeFunc {
	e := &effect{fn: fn, deps: map[source]struct{}{}}
	e.run()
	return e.dispose
}

func (e *effect) run() {
	runtime.Lock()
	if e.disposed || e.running {
		runtime.Unlock()
		return
	}
	e.running = true
	for dep := range e.deps {
		dep.unsubscribeLocked(e)
	}
	e.deps = map[source]struct{}{}
	prev := runtime.current
	runtime.current = e
	runtime.Unlock()

	defer func() {
		runtime.Lock()
		runtime.current = prev
		e.running = false
		runtime.Unlock()
	}()

	e.fn()
}

func (e *effect) dispose() {
	runtime.Lock()
	defer runtime.Unlock()
	if e.disposed {
		return
	}
	e.disposed = true
	for dep := range e.deps {
		dep.unsubscribeLocked(e)
	}
	e.deps = nil
}
