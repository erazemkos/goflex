package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
)

// ParamFunc returns the decoded value for a named path parameter.
type ParamFunc func(name string) string

// DecodeRequest fills req from JSON body fields, path tags, and query tags.
// Fields tagged path:"name" are read from params, fields tagged query:"name"
// are read from URL query values, and all other exported fields follow normal
// encoding/json body decoding rules.
func DecodeRequest[Req any](r *http.Request, params ParamFunc, req *Req) error {
	if r == nil || req == nil {
		return nil
	}
	if hasBody(r.Method) && r.Body != nil && r.ContentLength != 0 {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(req); err != nil {
			return err
		}
	}
	rv := reflect.ValueOf(req)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return nil
	}
	rt := rv.Type()
	query := r.URL.Query()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		value := rv.Field(i)
		if !value.CanSet() {
			continue
		}
		if tag := field.Tag.Get("path"); tag != "" {
			if params == nil {
				continue
			}
			if err := setString(value, params(tag)); err != nil {
				return fmt.Errorf("path %s: %w", tag, err)
			}
			continue
		}
		if tag := field.Tag.Get("query"); tag != "" {
			if !query.Has(tag) {
				continue
			}
			if err := setString(value, query.Get(tag)); err != nil {
				return fmt.Errorf("query %s: %w", tag, err)
			}
		}
	}
	return nil
}

func hasBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func setString(v reflect.Value, raw string) error {
	if v.Kind() == reflect.Pointer {
		if raw == "" {
			return nil
		}
		v.Set(reflect.New(v.Type().Elem()))
		return setString(v.Elem(), raw)
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(raw, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetUint(u)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetFloat(f)
	default:
		return fmt.Errorf("unsupported field type %s", v.Type())
	}
	return nil
}
