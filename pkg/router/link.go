package router

import (
	"reflect"
	"strings"

	"github.com/erazemkos/goflex/pkg/ui"
)

// Link renders an anchor that performs client-side navigation for ordinary left clicks.
func Link(href string, children ...ui.Element) ui.Element {
	args := []any{ui.Href(href), ui.OnClick(func(ev ui.Event) {
		if shouldHandleLinkClick(ev) {
			preventDefault(ev)
			UseNavigate()(href)
		}
	})}
	for _, c := range children {
		args = append(args, c)
	}
	return ui.A(args...)
}

func shouldHandleLinkClick(ev ui.Event) bool {
	if eventBool(ev.Value, "DefaultPrevented", "defaultPrevented") {
		return false
	}
	if eventInt(ev.Value, "Button", "button") != 0 {
		return false
	}
	if eventBool(ev.Value, "CtrlKey", "ctrlKey") || eventBool(ev.Value, "MetaKey", "metaKey") || eventBool(ev.Value, "ShiftKey", "shiftKey") || eventBool(ev.Value, "AltKey", "altKey") {
		return false
	}
	return true
}

func preventDefault(ev ui.Event) {
	if ev.Value == nil {
		return
	}
	if p, ok := ev.Value.(interface{ PreventDefault() }); ok {
		p.PreventDefault()
		return
	}
	v := reflect.ValueOf(ev.Value)
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		field := v.Elem().FieldByName("Prevented")
		if field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
			field.SetBool(true)
		}
	}
}

func eventBool(v any, names ...string) bool {
	if v == nil {
		return false
	}
	if m, ok := v.(map[string]any); ok {
		for _, name := range names {
			if b, ok := m[name].(bool); ok {
				return b
			}
		}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return false
	}
	for _, name := range names {
		field := rv.FieldByNameFunc(func(fieldName string) bool { return strings.EqualFold(fieldName, name) })
		if field.IsValid() && field.Kind() == reflect.Bool {
			return field.Bool()
		}
	}
	return false
}

func eventInt(v any, names ...string) int {
	if v == nil {
		return 0
	}
	if m, ok := v.(map[string]any); ok {
		for _, name := range names {
			switch n := m[name].(type) {
			case int:
				return n
			case float64:
				return int(n)
			}
		}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return 0
	}
	for _, name := range names {
		field := rv.FieldByNameFunc(func(fieldName string) bool { return strings.EqualFold(fieldName, name) })
		if field.IsValid() {
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return int(field.Int())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return int(field.Uint())
			}
		}
	}
	return 0
}
