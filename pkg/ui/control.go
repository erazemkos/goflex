package ui

import "fmt"

func Fragment(children ...Element) Element {
	return Element{kind: kindFragment, children: children, props: map[string]any{}, events: map[string]func(Event){}}
}
func If(cond bool, a, b Element) Element {
	if cond {
		return a
	}
	return b
}
func When(cond bool, a Element) Element {
	if cond {
		return a
	}
	return Fragment()
}
func For[T any](xs []T, key func(T) string, fn func(T) Element) Element {
	seen := map[string]bool{}
	out := make([]Element, 0, len(xs))
	for _, x := range xs {
		k := key(x)
		if DevMode && seen[k] {
			panic(fmt.Sprintf("duplicate key %q", k))
		}
		seen[k] = true
		e := fn(x)
		if e.props == nil {
			e.props = map[string]any{}
		}
		e.props["key"] = k
		out = append(out, e)
	}
	return Fragment(out...)
}
