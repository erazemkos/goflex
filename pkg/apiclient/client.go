package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/goflex/goflex/pkg/api"
	"github.com/goflex/goflex/pkg/httperr"
)

var BaseURL = ""
var Client = http.DefaultClient

type Code string

func (c Code) Error() string { return string(c) }

type FieldError struct {
	Code    Code
	Message string
	Fields  map[string]string
}

func (e FieldError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e FieldError) Is(target error) bool {
	if c, ok := target.(Code); ok {
		return e.Code == c
	}
	var c Code
	if errors.As(target, &c) {
		return e.Code == c
	}
	return false
}

func Call[Req, Res any](ctx context.Context, ep api.Endpoint[Req, Res], req Req) (Res, error) {
	var zero Res
	u, body, err := BuildRequest(ep.Method, ep.Path, req)
	if err != nil {
		return zero, err
	}
	var reader io.Reader
	if body != nil {
		reader = body
	}
	httpReq, err := http.NewRequestWithContext(ctx, ep.Method, strings.TrimRight(BaseURL, "/")+"/api"+u, reader)
	if err != nil {
		return zero, err
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := Client.Do(httpReq)
	if err != nil {
		return zero, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return zero, decodeError(resp)
	}
	if resp.StatusCode == http.StatusNoContent {
		return zero, nil
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return zero, nil
	}
	if err := json.Unmarshal(b, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

func BuildRequest[Req any](method, path string, req Req) (string, *bytes.Reader, error) {
	vals := url.Values{}
	rv := reflect.ValueOf(req)
	rt := reflect.TypeOf(req)
	bodyMap := map[string]any{}
	if !rv.IsValid() {
		return path, nil, nil
	}
	if rt.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return path, nil, nil
		}
		rv = rv.Elem()
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		if bodyful(method) {
			b, err := json.Marshal(req)
			if err != nil {
				return "", nil, err
			}
			return path, bytes.NewReader(b), nil
		}
		return path, nil, nil
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		v := rv.Field(i)
		if !v.CanInterface() {
			continue
		}
		if tag := f.Tag.Get("path"); tag != "" {
			path = strings.ReplaceAll(path, ":"+tag, url.PathEscape(fmt.Sprint(v.Interface())))
			continue
		}
		if tag := f.Tag.Get("query"); tag != "" {
			vals.Set(tag, fmt.Sprint(v.Interface()))
			continue
		}
		name := f.Tag.Get("json")
		if name == "" {
			name = strings.ToLower(f.Name[:1]) + f.Name[1:]
		} else {
			name = strings.Split(name, ",")[0]
		}
		if name != "-" {
			bodyMap[name] = v.Interface()
		}
	}
	if len(vals) > 0 {
		path += "?" + vals.Encode()
	}
	if !bodyful(method) || len(bodyMap) == 0 {
		return path, nil, nil
	}
	b, err := json.Marshal(bodyMap)
	if err != nil {
		return "", nil, err
	}
	return path, bytes.NewReader(b), nil
}

func decodeError(resp *http.Response) error {
	var he httperr.Error
	if err := json.NewDecoder(resp.Body).Decode(&he); err != nil && !errors.Is(err, io.EOF) {
		return FieldError{Code: Code(statusCode(resp.StatusCode, resp.Status)), Message: resp.Status}
	}
	if he.Code == "" {
		he.Code = statusCode(resp.StatusCode, resp.Status)
	}
	if he.Message == "" {
		he.Message = http.StatusText(resp.StatusCode)
	}
	return FieldError{Code: Code(he.Code), Message: he.Message, Fields: he.Fields}
}

func statusCode(status int, fallback string) string {
	text := strings.ToLower(http.StatusText(status))
	if text == "" {
		text = fallback
	}
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, " ", "_")
	if text == "" {
		return "error"
	}
	return text
}

func bodyful(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}
