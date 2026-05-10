package ui

import "fmt"

type FieldBinding interface {
	Name() string
	Value() any
	Error() string
	Set(any)
	Apply(*Element)
}

type SimpleField struct {
	N      string
	V      any
	Err    string
	Setter func(any)
}

func (f SimpleField) Name() string  { return f.N }
func (f SimpleField) Value() any    { return f.V }
func (f SimpleField) Error() string { return f.Err }
func (f SimpleField) Set(v any) {
	if f.Setter != nil {
		f.Setter(v)
	}
}
func (f SimpleField) Apply(e *Element) {
	Attr("name", f.N).Apply(e)
	Attr("value", f.V).Apply(e)
	OnChange(func(ev Event) { f.Set(ev.Value) }).Apply(e)
	OnBlur(func(ev Event) {
		if ev.Value != nil {
			f.Set(ev.Value)
		}
	}).Apply(e)
	if f.Err != "" {
		Attr("aria-invalid", "true").Apply(e)
		Attr("aria-describedby", fmt.Sprintf("%s-error", f.N)).Apply(e)
	}
}

// FieldError renders accessible error text for a field binding.
func FieldError(binding FieldBinding) Element {
	if binding == nil || binding.Error() == "" {
		return Fragment()
	}
	return Span(ID(fmt.Sprintf("%s-error", binding.Name())), Attr("role", "alert"), Text(binding.Error()))
}
