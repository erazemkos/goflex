package ui

import "reflect"

func Div(args ...any) Element    { return makeElement("div", args...) }
func Span(args ...any) Element   { return makeElement("span", args...) }
func H1(args ...any) Element     { return makeElement("h1", args...) }
func H2(args ...any) Element     { return makeElement("h2", args...) }
func H3(args ...any) Element     { return makeElement("h3", args...) }
func H4(args ...any) Element     { return makeElement("h4", args...) }
func H5(args ...any) Element     { return makeElement("h5", args...) }
func H6(args ...any) Element     { return makeElement("h6", args...) }
func P(args ...any) Element      { return makeElement("p", args...) }
func A(args ...any) Element      { return makeElement("a", args...) }
func Button(args ...any) Element { return makeElement("button", args...) }
func Input(args ...any) Element  { return makeElement("input", args...) }
func TextInput(binding FieldBinding, args ...any) Element {
	all := append([]any{binding, Type("text")}, args...)
	return Input(all...)
}
func EmailInput(binding FieldBinding, args ...any) Element {
	all := append([]any{binding, Type("email")}, args...)
	return Input(all...)
}
func PasswordInput(binding FieldBinding, args ...any) Element {
	all := append([]any{binding, Type("password")}, args...)
	return Input(all...)
}
func DateInput(binding FieldBinding, args ...any) Element {
	all := append([]any{binding, Type("date")}, args...)
	return Input(all...)
}
func Checkbox(binding FieldBinding, args ...any) Element {
	all := append([]any{binding, Type("checkbox"), Attr("checked", binding.Value())}, args...)
	return Input(all...)
}
func Radio(binding FieldBinding, value any, args ...any) Element {
	all := append([]any{binding, Type("radio"), Attr("value", value), Attr("checked", reflect.DeepEqual(binding.Value(), value))}, args...)
	return Input(all...)
}
func Textarea(args ...any) Element { return makeElement("textarea", args...) }
func Select[T any](binding FieldBinding, values ...T) Element {
	args := []any{binding}
	for _, v := range values {
		args = append(args, makeElement("option", Attr("value", v), Textf("%v", v)))
	}
	return makeElement("select", args...)
}
func Form(args ...any) Element    { return makeElement("form", args...) }
func Label(args ...any) Element   { return makeElement("label", args...) }
func Ul(args ...any) Element      { return makeElement("ul", args...) }
func Li(args ...any) Element      { return makeElement("li", args...) }
func Img(args ...any) Element     { return makeElement("img", args...) }
func Nav(args ...any) Element     { return makeElement("nav", args...) }
func Section(args ...any) Element { return makeElement("section", args...) }
func Article(args ...any) Element { return makeElement("article", args...) }
func Header(args ...any) Element  { return makeElement("header", args...) }
func Footer(args ...any) Element  { return makeElement("footer", args...) }
func NumberInput(binding FieldBinding, args ...any) Element {
	all := append([]any{binding, Type("number")}, args...)
	return Input(all...)
}
