package ui

import (
	"sort"
	"strings"
)

// Class appends one or more CSS class strings to an element's className prop.
func Class(classes ...string) Prop {
	return propFunc(func(e *Element) { addClass(e, strings.Join(classes, " ")) })
}

// ClassIf appends classes only when cond is true.
func ClassIf(cond bool, classes ...string) Prop {
	if !cond {
		return propFunc(func(e *Element) {
			if _, ok := e.props["className"]; !ok {
				e.props["className"] = ""
			}
		})
	}
	return Class(classes...)
}

// ClassMap appends all enabled classes from m in deterministic key order.
func ClassMap(m map[string]bool) Prop {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return Class(keys...)
}

func addClass(e *Element, c string) {
	c = strings.Join(strings.Fields(c), " ")
	if c == "" {
		return
	}
	old, _ := e.props["className"].(string)
	if old != "" {
		e.props["className"] = old + " " + c
	} else {
		e.props["className"] = c
	}
}

// Tw merges Tailwind utility classes with later conflicting utilities winning.
// It intentionally implements the common tailwind-merge groups used by GoFlex
// components without requiring a JavaScript dependency at runtime.
func Tw(parts ...string) string {
	tokens := make([]twToken, 0)
	lastByGroup := map[string]int{}
	removed := map[int]bool{}
	for _, part := range parts {
		for _, class := range strings.Fields(part) {
			group := twGroup(class)
			idx := len(tokens)
			tokens = append(tokens, twToken{class: class, group: group})
			if group == "" {
				continue
			}
			if old, ok := lastByGroup[group]; ok {
				removed[old] = true
			}
			lastByGroup[group] = idx
		}
	}
	out := make([]string, 0, len(tokens))
	for i, tok := range tokens {
		if !removed[i] {
			out = append(out, tok.class)
		}
	}
	return strings.Join(out, " ")
}

type twToken struct {
	class string
	group string
}

func twGroup(class string) string {
	mods, base := twSplitModifiers(class)
	important := ""
	if strings.HasPrefix(base, "!") {
		important = "!"
		base = strings.TrimPrefix(base, "!")
	}
	base = strings.TrimPrefix(base, "-")
	group := twBaseGroup(base)
	if group == "" {
		return ""
	}
	return mods + important + group
}

func twSplitModifiers(class string) (string, string) {
	depth := 0
	lastColon := -1
	for i, r := range class {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				lastColon = i
			}
		}
	}
	if lastColon == -1 {
		return "", class
	}
	return class[:lastColon+1], class[lastColon+1:]
}

func twBaseGroup(base string) string {
	if g := twExactGroup(base); g != "" {
		return g
	}
	if g := twSpacingGroup(base); g != "" {
		return g
	}
	if g := twSizingGroup(base); g != "" {
		return g
	}
	if g := twTextGroup(base); g != "" {
		return g
	}
	if g := twBorderGroup(base); g != "" {
		return g
	}
	for _, prefix := range []string{
		"bg-", "from-", "via-", "to-", "font-", "leading-", "tracking-",
		"rounded-tl-", "rounded-tr-", "rounded-br-", "rounded-bl-", "rounded-t-", "rounded-r-", "rounded-b-", "rounded-l-", "rounded-",
		"shadow", "opacity-", "overflow-x-", "overflow-y-", "overflow-", "z-",
		"top-", "right-", "bottom-", "left-", "inset-x-", "inset-y-", "inset-",
		"gap-x-", "gap-y-", "gap-", "space-x-", "space-y-", "ring-offset-", "ring-", "outline-",
		"cursor-", "select-", "object-", "aspect-", "order-", "basis-", "grow", "shrink",
	} {
		if base == strings.TrimSuffix(prefix, "-") || strings.HasPrefix(base, prefix) {
			return prefix
		}
	}
	return ""
}

func twExactGroup(base string) string {
	switch base {
	case "block", "inline-block", "inline", "flex", "inline-flex", "grid", "inline-grid", "table", "hidden":
		return "display"
	case "static", "fixed", "absolute", "relative", "sticky":
		return "position"
	case "visible", "invisible", "collapse":
		return "visibility"
	case "flex-row", "flex-row-reverse", "flex-col", "flex-col-reverse":
		return "flex-direction"
	case "flex-wrap", "flex-wrap-reverse", "flex-nowrap":
		return "flex-wrap"
	case "items-start", "items-end", "items-center", "items-baseline", "items-stretch":
		return "items"
	case "justify-normal", "justify-start", "justify-end", "justify-center", "justify-between", "justify-around", "justify-evenly", "justify-stretch":
		return "justify"
	case "content-normal", "content-center", "content-start", "content-end", "content-between", "content-around", "content-evenly", "content-baseline", "content-stretch":
		return "content"
	case "self-auto", "self-start", "self-end", "self-center", "self-stretch", "self-baseline":
		return "self"
	case "float-start", "float-end", "float-right", "float-left", "float-none":
		return "float"
	case "clear-start", "clear-end", "clear-left", "clear-right", "clear-both", "clear-none":
		return "clear"
	case "italic", "not-italic":
		return "font-style"
	case "underline", "overline", "line-through", "no-underline":
		return "text-decoration-line"
	case "uppercase", "lowercase", "capitalize", "normal-case":
		return "text-transform"
	case "truncate", "text-ellipsis", "text-clip":
		return "text-overflow"
	case "antialiased", "subpixel-antialiased":
		return "font-smoothing"
	}
	return ""
}

func twSpacingGroup(base string) string {
	for _, prefix := range []string{"px-", "py-", "pt-", "pr-", "pb-", "pl-", "p-", "mx-", "my-", "mt-", "mr-", "mb-", "ml-", "m-"} {
		if strings.HasPrefix(base, prefix) {
			return prefix
		}
	}
	return ""
}

func twSizingGroup(base string) string {
	for _, prefix := range []string{"min-w-", "max-w-", "w-", "min-h-", "max-h-", "h-"} {
		if strings.HasPrefix(base, prefix) {
			return prefix
		}
	}
	return ""
}

func twTextGroup(base string) string {
	if !strings.HasPrefix(base, "text-") {
		return ""
	}
	value := strings.TrimPrefix(base, "text-")
	switch value {
	case "left", "center", "right", "justify", "start", "end":
		return "text-align"
	case "xs", "sm", "base", "lg", "xl", "2xl", "3xl", "4xl", "5xl", "6xl", "7xl", "8xl", "9xl":
		return "text-size"
	}
	if strings.HasPrefix(value, "opacity-") {
		return "text-opacity"
	}
	return "text-color"
}

func twBorderGroup(base string) string {
	if base == "border" {
		return "border-width"
	}
	if strings.HasPrefix(base, "border-") {
		value := strings.TrimPrefix(base, "border-")
		switch value {
		case "0", "2", "4", "8":
			return "border-width"
		case "x", "x-0", "x-2", "x-4", "x-8", "y", "y-0", "y-2", "y-4", "y-8", "t", "t-0", "t-2", "t-4", "t-8", "r", "r-0", "r-2", "r-4", "r-8", "b", "b-0", "b-2", "b-4", "b-8", "l", "l-0", "l-2", "l-4", "l-8":
			return "border-width-" + strings.Split(value, "-")[0]
		case "solid", "dashed", "dotted", "double", "hidden", "none":
			return "border-style"
		}
		if strings.HasPrefix(value, "opacity-") {
			return "border-opacity"
		}
		return "border-color"
	}
	return ""
}
