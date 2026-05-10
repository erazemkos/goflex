package hooks

import "github.com/goflex/goflex/pkg/ui"

// Context stores a typed default value and the current provider value.
type Context[T any] struct {
	def     T
	current *T
}

// CreateContext creates a typed context with a default value.
func CreateContext[T any](def T) *Context[T] { return &Context[T]{def: def} }

// Provider sets the current value and returns a fragment containing children.
func (c *Context[T]) Provider(value T, children ...ui.Element) ui.Element {
	c.current = &value
	return ui.Fragment(children...)
}

// Clear removes the active provider value so UseContext returns the default.
func (c *Context[T]) Clear() { c.current = nil }

// WithProvider scopes a context value while fn renders. It is useful for tests and mock runtimes.
func (c *Context[T]) WithProvider(value T, fn func() ui.Element) ui.Element {
	old := c.current
	c.current = &value
	defer func() { c.current = old }()
	return fn()
}

// UseContext returns the nearest provider value or the context default.
func UseContext[T any](ctx *Context[T]) T {
	if ctx.current != nil {
		return *ctx.current
	}
	return ctx.def
}
