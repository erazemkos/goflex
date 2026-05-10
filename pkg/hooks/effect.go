package hooks

// UseEffect runs fn after render when dependencies change. The returned cleanup
// runs before the next changed effect and when the fiber unmounts.
func UseEffect(fn func() (cleanup func()), deps ...any) {
	f, idx, slot := claimSlot("effect")
	warnNonComparableDeps(f, deps)
	changed := !slot.initialized || depsChanged(slot.deps, deps)
	if !changed {
		return
	}
	oldCleanup := slot.cleanup
	if oldCleanup != nil {
		oldCleanup()
	}
	cleanup := fn()
	runtimeState.Lock()
	f.slots[idx] = hookSlot{kind: "effect", initialized: true, deps: copyDeps(deps), cleanup: cleanup}
	runtimeState.Unlock()
}
