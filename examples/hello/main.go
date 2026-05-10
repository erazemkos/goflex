//go:build js || gopherjs

package main

import "github.com/gopherjs/gopherjs/js"

func main() {
	react := js.Global.Get("React")
	reactDOM := js.Global.Get("ReactDOM")

	el := react.Call("createElement", "h1", nil, "Hello from Go")
	root := reactDOM.Call("createRoot", js.Global.Get("document").Call("getElementById", "root"))
	root.Call("render", el)
}
