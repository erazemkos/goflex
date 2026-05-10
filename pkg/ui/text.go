package ui

import "fmt"

func Text(s string) Element                    { return Element{kind: kindText, text: s} }
func Textf(format string, args ...any) Element { return Text(fmt.Sprintf(format, args...)) }

// TextFunc creates a fine-grained reactive text binding. Runtimes that support
// ReactiveTextRuntime can update the text node in place whenever signals read
// by fn change; other runtimes render fn's current value as static text.
func TextFunc(fn func() string) Element {
	if fn == nil {
		fn = func() string { return "" }
	}
	return Element{kind: kindTextFunc, textFunc: fn}
}
