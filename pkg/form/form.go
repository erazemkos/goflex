package form

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/goflex/goflex/pkg/httperr"
	"github.com/goflex/goflex/pkg/ui"
)

type Mode int

const (
	OnChange Mode = iota
	OnSubmitOnly
)

type options struct {
	mode       Mode
	validators CustomValidators
}

type Opt func(*options)

func WithMode(m Mode) Opt { return func(o *options) { o.mode = m } }
func WithValidator(field string, fn CustomValidator) Opt {
	return func(o *options) {
		if o.validators == nil {
			o.validators = CustomValidators{}
		}
		o.validators[field] = fn
	}
}
func WithValidators(validators CustomValidators) Opt {
	return func(o *options) {
		if o.validators == nil {
			o.validators = CustomValidators{}
		}
		for field, fn := range validators {
			o.validators[field] = fn
		}
	}
}

type Form[T any] struct {
	mu         sync.Mutex
	initial    T
	value      T
	errors     map[string]string
	touched    map[string]bool
	submitting bool
	opts       options
}

func UseForm[T any](initial T, opts ...Opt) *Form[T] {
	o := options{mode: OnChange, validators: CustomValidators{}}
	for _, fn := range opts {
		fn(&o)
	}
	return &Form[T]{initial: initial, value: initial, errors: map[string]string{}, touched: map[string]bool{}, opts: o}
}

func (f *Form[T]) Value() T {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value
}

func (f *Form[T]) IsValid() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.errors) == 0
}

func (f *Form[T]) IsDirty() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !reflect.DeepEqual(f.value, f.initial)
}

func (f *Form[T]) IsSubmitting() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.submitting
}

func (f *Form[T]) FieldError(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fieldErrorLocked(name)
}

func (f *Form[T]) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.value = f.initial
	f.errors = map[string]string{}
	f.touched = map[string]bool{}
	f.submitting = false
}

func (f *Form[T]) Set(name string, value any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	field, info, ok := f.fieldLocked(name)
	if !ok {
		unknownField(name)
		return
	}
	if err := setValue(field, value); err != nil {
		if ui.DevMode {
			panic(fmt.Sprintf("invalid value for form field %s: %v", name, err))
		}
		return
	}
	f.touched[fieldKey(info)] = true
	if f.opts.mode != OnSubmitOnly {
		f.validateLocked()
	}
}

func (f *Form[T]) Field(name string) ui.FieldBinding {
	f.mu.Lock()
	defer f.mu.Unlock()
	value := f.fieldValueLocked(name)
	return ui.SimpleField{N: name, V: value, Err: f.fieldErrorLocked(name), Setter: func(v any) { f.Set(name, v) }}
}

func (f *Form[T]) Submit(onSubmit func(T) error) func() {
	return func() {
		f.mu.Lock()
		f.validateLocked()
		if len(f.errors) > 0 {
			f.mu.Unlock()
			return
		}
		f.submitting = true
		v := f.value
		f.mu.Unlock()

		err := onSubmit(v)

		f.mu.Lock()
		defer f.mu.Unlock()
		f.submitting = false
		if err != nil {
			f.mergeErrorLocked(err)
		}
	}
}

func (f *Form[T]) validateLocked() {
	f.errors = ValidateWith(f.value, f.opts.validators)
	if f.errors == nil {
		f.errors = map[string]string{}
	}
}

func (f *Form[T]) mergeErrorLocked(err error) {
	if f.errors == nil {
		f.errors = map[string]string{}
	}
	var ptr *httperr.Error
	if errors.As(err, &ptr) && ptr != nil {
		for k, v := range ptr.Fields {
			f.errors[k] = v
		}
		return
	}
	// Accept FieldError-like values without importing the API client package.
	rv := derefValue(reflect.ValueOf(err))
	if rv.IsValid() && rv.Kind() == reflect.Struct {
		field := rv.FieldByName("Fields")
		if field.IsValid() && field.CanInterface() {
			if fields, ok := field.Interface().(map[string]string); ok {
				for k, v := range fields {
					f.errors[k] = v
				}
			}
		}
	}
}

func (f *Form[T]) fieldErrorLocked(name string) string {
	if f.errors == nil {
		return ""
	}
	if err := f.errors[name]; err != "" {
		return err
	}
	if _, info, ok := f.fieldLocked(name); ok {
		if err := f.errors[fieldKey(info)]; err != "" {
			return err
		}
		if err := f.errors[info.Name]; err != "" {
			return err
		}
	}
	return ""
}

func (f *Form[T]) fieldValueLocked(name string) any {
	field, _, ok := f.fieldLocked(name)
	if ok && field.CanInterface() {
		return field.Interface()
	}
	if !ok {
		unknownField(name)
	}
	return nil
}

func (f *Form[T]) fieldLocked(name string) (reflect.Value, reflect.StructField, bool) {
	rv := derefValue(reflect.ValueOf(&f.value))
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return reflect.Value{}, reflect.StructField{}, false
	}
	rt := rv.Type()
	if sf, ok := rt.FieldByName(name); ok {
		return rv.FieldByIndex(sf.Index), sf, true
	}
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if fieldKey(sf) == name {
			return rv.Field(i), sf, true
		}
	}
	return reflect.Value{}, reflect.StructField{}, false
}

func unknownField(name string) {
	if ui.DevMode {
		panic("unknown form field " + name)
	}
}

func setValue(field reflect.Value, value any) error {
	if !field.IsValid() || !field.CanSet() {
		return fmt.Errorf("field cannot be set")
	}
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	vv := reflect.ValueOf(value)
	if vv.Type().AssignableTo(field.Type()) {
		field.Set(vv)
		return nil
	}
	if vv.Type().ConvertibleTo(field.Type()) && vv.Kind() != reflect.String {
		field.Set(vv.Convert(field.Type()))
		return nil
	}
	if s, ok := value.(string); ok {
		return setString(field, s)
	}
	return fmt.Errorf("%s is not assignable to %s", vv.Type(), field.Type())
}

func setString(field reflect.Value, raw string) error {
	if field.Kind() == reflect.Pointer {
		if raw == "" {
			field.Set(reflect.Zero(field.Type()))
			return nil
		}
		field.Set(reflect.New(field.Type().Elem()))
		return setString(field.Elem(), raw)
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		if raw == "on" {
			field.SetBool(true)
			return nil
		}
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(u)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(v)
	default:
		return fmt.Errorf("cannot convert string to %s", field.Type())
	}
	return nil
}
