// Package reactive provides fine-grained signals and effects for browser-side
// state. Effects subscribe to the signals they read and re-run only when those
// signals change, which lets UI bindings update local DOM nodes without a
// global render pass or virtual-DOM diff.
//
// The runtime is intended for browser/UI event-loop use. It is safe for normal
// event callbacks and promise callbacks, but it is not designed as a concurrent
// server-side state container shared by multiple goroutines.
package reactive
