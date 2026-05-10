// Package browser provides small GopherJS browser helpers used by GoFlex
// frontend code.
package browser

import (
	"github.com/erazemkos/goflex/pkg/reactive"
	"github.com/gopherjs/gopherjs/js"
)

// JSON is the JavaScript object returned by response.json().
type JSON = js.Object

// ElementID accepts project-local typed string ID aliases.
type ElementID interface{ ~string }

// ByID returns document.getElementById(id).
func ByID[T ElementID](id T) *js.Object {
	return js.Global.Get("document").Call("getElementById", string(id))
}

// SetText sets textContent on an element by ID.
func SetText[T ElementID](id T, text string) {
	ByID(id).Set("textContent", text)
}

// BindText creates a fine-grained text binding. The text function subscribes to
// any reactive signals it reads, and only this element's textContent is updated
// when those signals change.
func BindText[T ElementID](id T, text func() string) reactive.DisposeFunc {
	return reactive.Effect(func() {
		SetText(id, text())
	})
}

// OnClick registers a click listener for an element by ID.
func OnClick[T ElementID](id T, fn func()) {
	ByID(id).Call("addEventListener", "click", func() { fn() })
}

// OnInput registers an input listener and passes event.target.value to fn.
func OnInput[T ElementID](id T, fn func(value string)) {
	ByID(id).Call("addEventListener", "input", func(event *js.Object) {
		fn(event.Get("target").Get("value").String())
	})
}

// EncodeURIComponent returns JavaScript's encodeURIComponent(value).
func EncodeURIComponent(value string) string {
	return js.Global.Call("encodeURIComponent", value).String()
}

// FetchJSON fetches path, decodes the JSON response, stores it in value, and
// writes any network/status error message to errText. It intentionally
// does not expose a loading signal; callers can leave dependent UI blank until
// the first value or error arrives.
func FetchJSON[T any](path string, decode func(*JSON) T, value *reactive.Signal[*T], errText *reactive.Signal[string]) {
	FetchJSONFunc(path, func(data *JSON) {
		decoded := decode(data)
		value.Set(&decoded)
		errText.Set("")
	}, func(message string) {
		errText.Set(message)
	})
}

// FetchJSONFunc is the lower-level callback form of FetchJSON.
func FetchJSONFunc(path string, onOK func(*JSON), onErr func(string)) {
	js.Global.Call("fetch", path).
		Call("then", func(resp *js.Object) {
			if !resp.Get("ok").Bool() {
				onErr("API returned " + resp.Get("status").String())
				return
			}
			resp.Call("json").
				Call("then", func(data *js.Object) { onOK(data) }).
				Call("catch", func(err *js.Object) { onErr(err.String()) })
		}).
		Call("catch", func(err *js.Object) { onErr(err.String()) })
}
