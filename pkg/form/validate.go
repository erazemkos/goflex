package form

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// CustomValidator validates one field. Return an empty string for success or a
// user-facing error message/code for failure.
type CustomValidator func(value any, whole any) string

// CustomValidators maps either struct field names ("Title") or json names
// ("title") to field validators.
type CustomValidators map[string]CustomValidator

// Validate applies validate tags using go-playground/validator. Error map keys
// are JSON field names when available, falling back to struct field names. The
// same function is used on the client and server for validation parity.
func Validate(v any) map[string]string { return ValidateWith(v, nil) }

// ValidateWith applies tag-driven validation plus optional per-field custom
// validators.
func ValidateWith(v any, custom CustomValidators) map[string]string {
	out := map[string]string{}
	if err := validate.Struct(v); err != nil {
		if ves, ok := err.(validator.ValidationErrors); ok {
			rt := derefType(reflect.TypeOf(v))
			for _, fe := range ves {
				name := fe.StructField()
				if rt.Kind() == reflect.Struct {
					if f, ok := rt.FieldByName(name); ok {
						name = fieldKey(f)
					}
				}
				out[name] = validationMessage(fe)
			}
		}
	}
	if len(custom) > 0 {
		rv := derefValue(reflect.ValueOf(v))
		if rv.IsValid() && rv.Kind() == reflect.Struct {
			rt := rv.Type()
			for i := 0; i < rt.NumField(); i++ {
				field := rt.Field(i)
				value := rv.Field(i)
				if !value.CanInterface() {
					continue
				}
				jsonName := fieldKey(field)
				for _, key := range []string{field.Name, jsonName} {
					fn := custom[key]
					if fn == nil {
						continue
					}
					if msg := fn(value.Interface(), v); msg != "" {
						out[jsonName] = msg
					}
					break
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "required"
	case "email":
		return "email"
	case "min":
		return fmt.Sprintf("min=%s", fe.Param())
	case "max":
		return fmt.Sprintf("max=%s", fe.Param())
	case "oneof":
		return fmt.Sprintf("oneof=%s", fe.Param())
	default:
		if fe.Param() != "" {
			return fe.Tag() + "=" + fe.Param()
		}
		return fe.Tag()
	}
}

func fieldKey(f reflect.StructField) string {
	if jsonName := stringsBeforeComma(f.Tag.Get("json")); jsonName != "" && jsonName != "-" {
		return jsonName
	}
	return f.Name
}

func derefType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func derefValue(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func stringsBeforeComma(s string) string {
	if i := strings.IndexByte(s, ','); i >= 0 {
		return s[:i]
	}
	return s
}
