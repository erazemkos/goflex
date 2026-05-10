package uitest

import "github.com/goflex/goflex/pkg/ui"

// MockNode mirrors a rendered UI tree without requiring GopherJS or React.
type MockNode struct {
	Tag      string         `json:"tag,omitempty"`
	Text     string         `json:"text,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Children []MockNode     `json:"children,omitempty"`
	Raw      any            `json:"-"`
}

// Runtime records calls made by ui.Render.
type Runtime struct {
	MountedContainer any
	MountedElement   any
}

func (r *Runtime) CreateElement(tag string, props map[string]any, children ...any) any {
	n := MockNode{Tag: tag, Props: props}
	for _, child := range children {
		n.Children = append(n.Children, child.(MockNode))
	}
	return n
}

func (r *Runtime) CreateFragment(children ...any) any {
	n := MockNode{}
	for _, child := range children {
		n.Children = append(n.Children, child.(MockNode))
	}
	return n
}

func (r *Runtime) CreateText(text string) any { return MockNode{Text: text} }
func (r *Runtime) UseRaw(value any) any       { return MockNode{Raw: value} }
func (r *Runtime) Mount(container any, element any) {
	r.MountedContainer = container
	r.MountedElement = element
}

// Render returns a MockNode mirroring the ui.Element tree.
func Render(e ui.Element) MockNode {
	rt := &Runtime{}
	return ui.Render(e, rt).(MockNode)
}
