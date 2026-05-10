package ui

type Prop interface{ Apply(*Element) }
type propFunc func(*Element)

func (f propFunc) Apply(e *Element) { f(e) }

func Attr(k string, v any) Prop {
	return propFunc(func(e *Element) {
		if e.props == nil {
			e.props = map[string]any{}
		}
		e.props[k] = v
	})
}
func ID(v string) Prop          { return Attr("id", v) }
func Style(v any) Prop          { return Attr("style", v) }
func Href(v string) Prop        { return Attr("href", v) }
func Src(v string) Prop         { return Attr("src", v) }
func Alt(v string) Prop         { return Attr("alt", v) }
func Type(v string) Prop        { return Attr("type", v) }
func Value(v any) Prop          { return Attr("value", v) }
func Placeholder(v string) Prop { return Attr("placeholder", v) }
func Disabled(v bool) Prop      { return Attr("disabled", v) }
func Name(v string) Prop        { return Attr("name", v) }
