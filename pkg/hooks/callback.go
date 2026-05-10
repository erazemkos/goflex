package hooks

// UseCallback returns the same function value until dependencies change.
func UseCallback(fn any, deps ...any) any {
	return UseMemo(func() any { return fn }, deps...)
}
