package ui

import "fmt"

var DevMode = true

type elementKind string

const (
	kindTag       elementKind = "tag"
	kindText      elementKind = "text"
	kindTextFunc  elementKind = "textFunc"
	kindFragment  elementKind = "fragment"
	kindComponent elementKind = "component"
	kindRaw       elementKind = "raw"
)

type Element struct {
	kind     elementKind
	tag      string
	props    map[string]any
	events   map[string]func(Event)
	children []Element
	text     string
	textFunc func() string
	comp     func(Props) Element
	raw      any
	name     string
}

type Event struct{ Value any }

type Props struct{ values map[string]any }

func (p Props) String(k string) string {
	v, ok := p.values[k]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
func (p Props) Int(k string) int {
	switch v := p.values[k].(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
func (p Props) Bool(k string) bool { v, _ := p.values[k].(bool); return v }
func (p Props) Any(k string) any   { return p.values[k] }

func makeElement(tag string, args ...any) Element {
	e := Element{kind: kindTag, tag: tag, props: map[string]any{}, events: map[string]func(Event){}}
	applyArgs(&e, args...)
	return e
}
func applyArgs(e *Element, args ...any) {
	for _, a := range args {
		switch v := a.(type) {
		case nil:
		case Element:
			e.children = append(e.children, v)
		case []Element:
			e.children = append(e.children, v...)
		case string:
			e.children = append(e.children, Text(v))
		case func() string:
			e.children = append(e.children, TextFunc(v))
		case EventProp:
			v.Apply(e)
		case FieldBinding:
			v.Apply(e)
		case Prop:
			v.Apply(e)
		default:
			e.children = append(e.children, Text(fmt.Sprint(v)))
		}
	}
}

func (e Element) Kind() string { return string(e.kind) }
func (e Element) Tag() string  { return e.tag }
func (e Element) TextValue() string {
	if e.kind == kindTextFunc && e.textFunc != nil {
		return e.textFunc()
	}
	return e.text
}
func (e Element) Props() map[string]any {
	out := map[string]any{}
	for k, v := range e.props {
		out[k] = v
	}
	return out
}
func (e Element) Events() map[string]func(Event) { return e.events }
func (e Element) Children() []Element            { return append([]Element(nil), e.children...) }
func (e Element) RawValue() any                  { return e.raw }

func Raw(v any) Element { return Element{kind: kindRaw, raw: v} }

// Runtime is the minimal adapter Render needs to target React or a mock runtime.
type Runtime interface {
	CreateElement(tag string, props map[string]any, children ...any) any
	CreateFragment(children ...any) any
	CreateText(text string) any
	UseRaw(value any) any
	Mount(container any, element any)
}

// ReactiveTextRuntime is an optional Runtime extension for fine-grained text
// bindings. A browser runtime can create one text node and use pkg/reactive to
// update only that node whenever fn's signal dependencies change. The runtime
// owns any effect disposal when that node is unmounted. Runtimes that do not
// implement this extension receive the current value as static text.
type ReactiveTextRuntime interface {
	CreateReactiveText(fn func() string) any
}

// MountTarget pairs a Runtime with the concrete DOM/container value it should mount into.
type MountTarget struct {
	Runtime   Runtime
	Container any
}

// Render walks root and emits runtime elements. Passing a MountTarget mounts the
// resulting runtime element into MountTarget.Container. Passing a Runtime uses a
// nil mount container. Passing anything else returns root unchanged, which keeps
// tests and non-browser code independent from GopherJS.
func Render(root Element, container any) any {
	switch target := container.(type) {
	case MountTarget:
		if target.Runtime == nil {
			return root
		}
		node := renderWithRuntime(root, target.Runtime)
		target.Runtime.Mount(target.Container, node)
		return node
	case Runtime:
		node := renderWithRuntime(root, target)
		target.Mount(nil, node)
		return node
	default:
		return root
	}
}

func renderWithRuntime(e Element, rt Runtime) any {
	switch e.kind {
	case kindText:
		return rt.CreateText(e.text)
	case kindTextFunc:
		if dynamic, ok := rt.(ReactiveTextRuntime); ok {
			return dynamic.CreateReactiveText(e.textFunc)
		}
		return rt.CreateText(e.TextValue())
	case kindRaw:
		return rt.UseRaw(e.raw)
	case kindFragment:
		children := renderChildren(e.children, rt)
		return rt.CreateFragment(children...)
	case kindComponent:
		if len(e.children) == 0 && e.comp != nil {
			return renderWithRuntime(e.comp(Props{values: e.props}), rt)
		}
		if len(e.children) == 1 {
			return renderWithRuntime(e.children[0], rt)
		}
		children := renderChildren(e.children, rt)
		return rt.CreateFragment(children...)
	case kindTag:
		children := renderChildren(e.children, rt)
		props := e.Props()
		for name, fn := range e.events {
			props[name] = fn
		}
		return rt.CreateElement(e.tag, props, children...)
	default:
		return rt.CreateFragment()
	}
}

func renderChildren(children []Element, rt Runtime) []any {
	out := make([]any, 0, len(children))
	for _, child := range children {
		out = append(out, renderWithRuntime(child, rt))
	}
	return out
}
