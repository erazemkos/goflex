package hooks

// Ref holds mutable data that does not request a re-render when changed.
type Ref[T any] struct{ Current T }

// UseRef returns a stable mutable reference for a component fiber.
func UseRef[T any](initial T) *Ref[T] {
	f, idx, slot := claimSlot("ref")
	if slot.initialized {
		return slot.value.(*Ref[T])
	}
	ref := &Ref[T]{Current: initial}
	runtimeState.Lock()
	f.slots[idx] = hookSlot{kind: "ref", initialized: true, value: ref}
	runtimeState.Unlock()
	return ref
}
