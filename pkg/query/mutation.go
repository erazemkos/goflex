package query

import (
	"context"
	"sync"
)

type Mutation[Req, Res any] struct {
	fn         func(context.Context, Req) (Res, error)
	mu         sync.RWMutex
	pending    int
	err        error
	data       Res
	onSuccess  []func(Res)
	onError    []func(Req, error)
	optimistic []func(Req)
}

type MOpt[Req, Res any] func(*Mutation[Req, Res])

func OnSuccess[Req, Res any](fn func(Res)) MOpt[Req, Res] {
	return func(m *Mutation[Req, Res]) { m.onSuccess = append(m.onSuccess, fn) }
}

func OnError[Req, Res any](fn func(Req, error)) MOpt[Req, Res] {
	return func(m *Mutation[Req, Res]) { m.onError = append(m.onError, fn) }
}

func Optimistic[Req, Res any](fn func(Req)) MOpt[Req, Res] {
	return func(m *Mutation[Req, Res]) { m.optimistic = append(m.optimistic, fn) }
}

func UseMutation[Req, Res any](fn func(context.Context, Req) (Res, error), opts ...MOpt[Req, Res]) *Mutation[Req, Res] {
	m := &Mutation[Req, Res]{fn: fn}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *Mutation[Req, Res]) Mutate(req Req) { go func() { _, _ = m.MutateAsync(req) }() }

func (m *Mutation[Req, Res]) MutateAsync(req Req) (Res, error) {
	var snap map[string]entry
	if len(m.optimistic) > 0 {
		snap = snapshot()
		for _, fn := range m.optimistic {
			fn(req)
		}
	}
	m.mu.Lock()
	m.pending++
	m.mu.Unlock()

	res, err := m.fn(context.Background(), req)

	m.mu.Lock()
	m.pending--
	m.data = res
	m.err = err
	m.mu.Unlock()

	if err != nil {
		if snap != nil {
			restore(snap)
		}
		for _, fn := range m.onError {
			fn(req, err)
		}
		return res, err
	}
	for _, fn := range m.onSuccess {
		fn(res)
	}
	return res, nil
}

func (m *Mutation[Req, Res]) Pending() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pending > 0
}

func (m *Mutation[Req, Res]) Err() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err
}

func (m *Mutation[Req, Res]) Data() Res {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data
}
