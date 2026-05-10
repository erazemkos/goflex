package hooks

// UseReducer stores reducer-managed state and returns the current state plus a dispatch function.
func UseReducer[S, A any](reducer func(S, A) S, init S) (S, func(A)) {
	s := UseState(init)
	return s.Get(), func(a A) { s.Update(func(old S) S { return reducer(old, a) }) }
}
