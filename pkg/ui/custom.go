package ui

import "sync"

type ComponentFunc func(map[string]any) Element

var componentRenderHook struct {
	sync.RWMutex
	fn func(name string) func()
}

// SetComponentRenderHook installs a render lifecycle hook used by pkg/hooks.
func SetComponentRenderHook(fn func(name string) func()) {
	componentRenderHook.Lock()
	componentRenderHook.fn = fn
	componentRenderHook.Unlock()
}

func beginComponentRender(name string) func() {
	componentRenderHook.RLock()
	fn := componentRenderHook.fn
	componentRenderHook.RUnlock()
	if fn == nil {
		return func() {}
	}
	end := fn(name)
	if end == nil {
		return func() {}
	}
	return end
}

func Component(name string, render func(Props) Element) ComponentFunc {
	return func(values map[string]any) Element {
		props := map[string]any{}
		for k, v := range values {
			props[k] = v
		}
		end := beginComponentRender(name)
		defer end()
		return Element{kind: kindComponent, name: name, comp: render, props: props, children: []Element{render(Props{values: props})}}
	}
}
