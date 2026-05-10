package ui

type EventProp interface {
	Apply(*Element)
	eventMarker()
}
type eventProp struct {
	name string
	fn   func(Event)
}

func (p eventProp) eventMarker() {}
func (p eventProp) Apply(e *Element) {
	if e.events == nil {
		e.events = map[string]func(Event){}
	}
	e.events[p.name] = p.fn
}

func event(name string, f any) EventProp {
	switch fn := f.(type) {
	case func():
		return eventProp{name, func(Event) { fn() }}
	case func(Event):
		return eventProp{name, fn}
	default:
		return eventProp{name, func(Event) {}}
	}
}
func OnClick(f any) EventProp   { return event("onClick", f) }
func OnChange(f any) EventProp  { return event("onChange", f) }
func OnInput(f any) EventProp   { return event("onInput", f) }
func OnSubmit(f any) EventProp  { return event("onSubmit", f) }
func OnKeyDown(f any) EventProp { return event("onKeyDown", f) }
func OnFocus(f any) EventProp   { return event("onFocus", f) }
func OnBlur(f any) EventProp    { return event("onBlur", f) }
