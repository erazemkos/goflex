package hooks

// State is a typed wrapper around a hook state slot.
type State[T any] struct {
	fiberID string
	idx     int
}

// UseState returns the current value slot and setter for a component render.
func UseState[T any](initial T) *State[T] {
	f, idx, slot := claimSlot("state")
	if !slot.initialized {
		slot.initialized = true
		slot.value = initial
		runtimeState.Lock()
		f.slots[idx] = slot
		runtimeState.Unlock()
	}
	return &State[T]{fiberID: f.id, idx: idx}
}

// Get returns the current state value.
func (s *State[T]) Get() T {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	f := runtimeState.fibers[s.fiberID]
	if f == nil || s.idx >= len(f.slots) {
		var zero T
		return zero
	}
	if v, ok := f.slots[s.idx].value.(T); ok {
		return v
	}
	var zero T
	return zero
}

// Set stores a new value and records that this fiber should re-render.
func (s *State[T]) Set(v T) {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	f := getFiberLocked(s.fiberID)
	for len(f.slots) <= s.idx {
		f.slots = append(f.slots, hookSlot{})
	}
	f.slots[s.idx].value = v
	f.slots[s.idx].initialized = true
	f.renderRequested = true
	f.setCalls = append(f.setCalls, v)
}

// Update applies fn to the previous value and records the functional setter.
func (s *State[T]) Update(fn func(T) T) {
	runtimeState.Lock()
	f := getFiberLocked(s.fiberID)
	var old T
	if s.idx < len(f.slots) {
		old, _ = f.slots[s.idx].value.(T)
	}
	newValue := fn(old)
	for len(f.slots) <= s.idx {
		f.slots = append(f.slots, hookSlot{})
	}
	f.slots[s.idx].value = newValue
	f.slots[s.idx].initialized = true
	f.renderRequested = true
	f.setCalls = append(f.setCalls, fn)
	runtimeState.Unlock()
}
