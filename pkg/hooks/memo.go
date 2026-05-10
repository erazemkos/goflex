package hooks

// UseMemo returns a cached value until dependencies change.
func UseMemo[T any](fn func() T, deps ...any) T {
	f, idx, slot := claimSlot("memo")
	warnNonComparableDeps(f, deps)
	if slot.initialized && !depsChanged(slot.deps, deps) {
		return slot.value.(T)
	}
	v := fn()
	runtimeState.Lock()
	f.slots[idx] = hookSlot{kind: "memo", initialized: true, deps: copyDeps(deps), value: v}
	runtimeState.Unlock()
	return v
}
