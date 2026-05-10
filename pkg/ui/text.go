package ui

import "fmt"

func Text(s string) Element                    { return Element{kind: kindText, text: s} }
func Textf(format string, args ...any) Element { return Text(fmt.Sprintf(format, args...)) }
