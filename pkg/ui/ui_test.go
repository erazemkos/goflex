package ui

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/erazemkos/goflex/pkg/reactive"
)

func TestBasicElementConstruction(t *testing.T) {
	div := Div(Class("x"))
	if div.Kind() != "tag" || div.Tag() != "div" {
		t.Fatalf("kind/tag = %s/%s", div.Kind(), div.Tag())
	}
	if got := div.Props()["className"]; got != "x" {
		t.Fatalf("className=%v", got)
	}

	h1 := H1(Text("hi"))
	children := h1.Children()
	if len(children) != 1 || children[0].Kind() != "text" || children[0].TextValue() != "hi" {
		t.Fatalf("bad h1 children: %#v", children)
	}
}

func TestNestedTree(t *testing.T) {
	tree := Div(Ul(Li(Text("a")), Li(Text("b")), Li(Text("c"))))
	if tree.Tag() != "div" || len(tree.Children()) != 1 {
		t.Fatalf("bad root: %#v", tree)
	}
	ul := tree.Children()[0]
	if ul.Tag() != "ul" || len(ul.Children()) != 3 {
		t.Fatalf("bad ul: %#v", ul)
	}
	for i, li := range ul.Children() {
		if li.Tag() != "li" || len(li.Children()) != 1 {
			t.Fatalf("bad li %d: %#v", i, li)
		}
	}
}

func TestMultipleClassesMerge(t *testing.T) {
	e := Div(Class("a"), Class("b", "c"))
	if got := e.Props()["className"]; got != "a b c" {
		t.Fatalf("className=%v", got)
	}
}

func TestEventsRegisterAsCallableClosures(t *testing.T) {
	called := 0
	e := Button(OnClick(func() { called++ }), OnChange(func(Event) { called += 10 }))
	if e.Events()["onClick"] == nil || e.Events()["onChange"] == nil {
		t.Fatalf("events missing: %#v", e.Events())
	}
	e.Events()["onClick"](Event{})
	e.Events()["onChange"](Event{Value: "x"})
	if called != 11 {
		t.Fatalf("called=%d", called)
	}
}

func TestAllEventHelpers(t *testing.T) {
	called := map[string]bool{}
	e := Div(
		OnInput(func() { called["input"] = true }),
		OnSubmit(func() { called["submit"] = true }),
		OnKeyDown(func() { called["keydown"] = true }),
		OnFocus(func() { called["focus"] = true }),
		OnBlur(func() { called["blur"] = true }),
	)
	for name, key := range map[string]string{"onInput": "input", "onSubmit": "submit", "onKeyDown": "keydown", "onFocus": "focus", "onBlur": "blur"} {
		e.Events()[name](Event{})
		if !called[key] {
			t.Fatalf("%s did not call", name)
		}
	}
}

func TestConditionalRendering(t *testing.T) {
	a, b := Text("a"), Text("b")
	if If(true, a, b).TextValue() != "a" || If(false, a, b).TextValue() != "b" {
		t.Fatal("If returned wrong branch")
	}
	if When(true, a).TextValue() != "a" {
		t.Fatal("When(true) returned wrong element")
	}
	if got := When(false, a); got.Kind() != "fragment" || len(got.Children()) != 0 {
		t.Fatalf("When(false)=%#v", got)
	}
}

func TestForRendersEachItemAndKeys(t *testing.T) {
	list := For([]int{1, 2, 3}, strconv.Itoa, func(i int) Element { return Text(strconv.Itoa(i)) })
	children := list.Children()
	if list.Kind() != "fragment" || len(children) != 3 {
		t.Fatalf("bad list: %#v", list)
	}
	for i, child := range children {
		want := strconv.Itoa(i + 1)
		if child.TextValue() != want || child.Props()["key"] != want {
			t.Fatalf("child %d = text %q key %v", i, child.TextValue(), child.Props()["key"])
		}
	}
}

func TestForDuplicateKeysPanicInDevMode(t *testing.T) {
	old := DevMode
	DevMode = true
	defer func() { DevMode = old }()
	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate key panic")
		}
	}()
	_ = For([]int{1, 1}, strconv.Itoa, func(i int) Element { return Text(strconv.Itoa(i)) })
}

func TestForDuplicateKeysAllowedOutsideDevMode(t *testing.T) {
	old := DevMode
	DevMode = false
	defer func() { DevMode = old }()
	list := For([]int{1, 1}, strconv.Itoa, func(i int) Element { return Text(strconv.Itoa(i)) })
	if len(list.Children()) != 2 {
		t.Fatalf("children=%d", len(list.Children()))
	}
}

func TestCustomComponentsAndProps(t *testing.T) {
	greeting := Component("Greeting", func(p Props) Element {
		return Text(fmt.Sprintf("Hello %s #%d ok=%t any=%v missing=%q", p.String("name"), p.Int("count"), p.Bool("ok"), p.Any("raw"), p.String("missing")))
	})
	e := greeting(map[string]any{"name": "World", "count": int64(7), "ok": true, "raw": "x"})
	if e.Kind() != "component" {
		t.Fatalf("kind=%s", e.Kind())
	}
	if got := e.Children()[0].TextValue(); got != `Hello World #7 ok=true any=x missing=""` {
		t.Fatalf("component text=%q", got)
	}
	props := e.Props()
	props["name"] = "mutated"
	if e.Children()[0].TextValue() != `Hello World #7 ok=true any=x missing=""` {
		t.Fatal("component output changed after props mutation")
	}
}

func TestTextFuncEvaluatesCurrentValue(t *testing.T) {
	count := reactive.NewSignal(2)
	e := TextFunc(func() string { return strconv.Itoa(count.Get() * 2) })
	if e.Kind() != "textFunc" || e.TextValue() != "4" {
		t.Fatalf("bad TextFunc element: kind=%s text=%q", e.Kind(), e.TextValue())
	}
	count.Set(3)
	if e.TextValue() != "6" {
		t.Fatalf("TextValue did not evaluate latest signal value: %q", e.TextValue())
	}
}

func TestTextFuncUsesReactiveRuntimeForLocalUpdates(t *testing.T) {
	count := reactive.NewSignal(1)
	rt := &recordReactiveTextRuntime{}
	node := Render(TextFunc(func() string { return strconv.Itoa(count.Get()) }), rt).(*recordReactiveTextNode)
	defer rt.dispose.Dispose()
	if node.Text != "1" || rt.textRuns != 1 {
		t.Fatalf("initial node=%#v runs=%d", node, rt.textRuns)
	}
	count.Set(2)
	if node.Text != "2" || rt.textRuns != 2 {
		t.Fatalf("signal update should mutate same text node, node=%#v runs=%d", node, rt.textRuns)
	}
	other := reactive.NewSignal("x")
	other.Set("y")
	if node.Text != "2" || rt.textRuns != 2 {
		t.Fatalf("unrelated signal re-rendered text node, node=%#v runs=%d", node, rt.textRuns)
	}
}

func TestTextFuncFallsBackToStaticText(t *testing.T) {
	count := reactive.NewSignal(1)
	rt := &recordRuntime{}
	node := Render(TextFunc(func() string { return strconv.Itoa(count.Get()) }), rt).(recordNode)
	if node.Text != "1" {
		t.Fatalf("initial static text=%q", node.Text)
	}
	count.Set(2)
	if node.Text != "1" {
		t.Fatalf("fallback runtime should not mutate already-rendered value: %q", node.Text)
	}
}

func TestFuncStringChildBecomesTextFunc(t *testing.T) {
	count := reactive.NewSignal(7)
	e := Span(func() string { return strconv.Itoa(count.Get()) })
	children := e.Children()
	if len(children) != 1 || children[0].Kind() != "textFunc" || children[0].TextValue() != "7" {
		t.Fatalf("bad func child: %#v", children)
	}
}

func TestRawRoundTrip(t *testing.T) {
	raw := &struct{ Name string }{"third-party"}
	e := Raw(raw)
	if e.Kind() != "raw" || e.RawValue() != raw {
		t.Fatalf("raw did not roundtrip: %#v", e.RawValue())
	}
}

func TestPropsHelpers(t *testing.T) {
	e := Input(
		ID("id1"),
		Style(map[string]string{"color": "red"}),
		Attr("data-x", 1),
		Href("/x"),
		Src("/img.png"),
		Alt("alt"),
		Type("text"),
		Value("value"),
		Placeholder("ph"),
		Disabled(true),
		Name("field"),
	)
	props := e.Props()
	want := map[string]any{"id": "id1", "data-x": 1, "href": "/x", "src": "/img.png", "alt": "alt", "type": "text", "value": "value", "placeholder": "ph", "disabled": true, "name": "field"}
	for k, v := range want {
		if props[k] != v {
			t.Fatalf("prop %s=%v want %v", k, props[k], v)
		}
	}
	if props["style"] == nil {
		t.Fatal("style missing")
	}
}

func TestClassHelpers(t *testing.T) {
	if got := Div(ClassIf(true, "a", "b")).Props()["className"]; got != "a b" {
		t.Fatalf("ClassIf true=%v", got)
	}
	if got := Div(ClassIf(false, "a")).Props()["className"]; got != "" {
		t.Fatalf("ClassIf false=%v", got)
	}
	if got := Div(ClassMap(map[string]bool{"b": true, "a": true, "c": false})).Props()["className"]; got != "a b" {
		t.Fatalf("ClassMap=%v", got)
	}
}

func TestTwMergeCases(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want string
	}{
		"px later wins":       {[]string{"px-2", "px-4"}, "px-4"},
		"different scopes":    {[]string{"p-2", "px-4"}, "p-2 px-4"},
		"text color":          {[]string{"text-red-500", "text-blue-500"}, "text-blue-500"},
		"unknown passthrough": {[]string{"custom", "px-1", "px-2"}, "custom px-2"},
		"margin axis":         {[]string{"mx-2 my-1", "mx-4"}, "my-1 mx-4"},
		"width":               {[]string{"w-4", "w-8"}, "w-8"},
		"height":              {[]string{"h-4", "h-8"}, "h-8"},
		"min width":           {[]string{"min-w-0", "min-w-full"}, "min-w-full"},
		"display":             {[]string{"block", "flex"}, "flex"},
		"position":            {[]string{"absolute", "relative"}, "relative"},
		"flex direction":      {[]string{"flex-row", "flex-col"}, "flex-col"},
		"items":               {[]string{"items-start", "items-center"}, "items-center"},
		"justify":             {[]string{"justify-start", "justify-between"}, "justify-between"},
		"text size distinct":  {[]string{"text-sm", "text-lg"}, "text-lg"},
		"text size and color": {[]string{"text-sm", "text-red-500"}, "text-sm text-red-500"},
		"background":          {[]string{"bg-red-500", "bg-blue-500"}, "bg-blue-500"},
		"font weight":         {[]string{"font-medium", "font-bold"}, "font-bold"},
		"rounded":             {[]string{"rounded", "rounded-lg"}, "rounded-lg"},
		"border width":        {[]string{"border", "border-2"}, "border-2"},
		"border color":        {[]string{"border-red-500", "border-blue-500"}, "border-blue-500"},
		"shadow":              {[]string{"shadow-sm", "shadow-lg"}, "shadow-lg"},
		"opacity":             {[]string{"opacity-50", "opacity-100"}, "opacity-100"},
		"modifier scoped":     {[]string{"hover:px-2", "px-4", "hover:px-6"}, "px-4 hover:px-6"},
		"important scoped":    {[]string{"!px-2", "px-4", "!px-8"}, "px-4 !px-8"},
		"arbitrary value":     {[]string{"px-[13px]", "px-4"}, "px-4"},
		"negative margin":     {[]string{"-mt-2", "mt-4"}, "mt-4"},
		"gap":                 {[]string{"gap-2", "gap-4"}, "gap-4"},
		"overflow":            {[]string{"overflow-hidden", "overflow-auto"}, "overflow-auto"},
		"z index":             {[]string{"z-10", "z-20"}, "z-20"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Tw(tc.in...); got != tc.want {
				t.Fatalf("Tw=%q want %q", got, tc.want)
			}
		})
	}
}

func TestPrimitiveConstructors(t *testing.T) {
	constructors := map[string]func(...any) Element{
		"span": Span, "h2": H2, "h3": H3, "h4": H4, "h5": H5, "h6": H6,
		"p": P, "a": A, "form": Form, "label": Label, "img": Img, "nav": Nav,
		"section": Section, "article": Article, "header": Header, "footer": Footer,
		"textarea": Textarea,
	}
	for tag, fn := range constructors {
		if got := fn(Text("x")).Tag(); got != tag {
			t.Fatalf("%s constructor tag=%s", tag, got)
		}
	}
}

func TestFormBindingsAndSelectHelpers(t *testing.T) {
	var set any
	binding := SimpleField{N: "Priority", V: 2, Err: "bad", Setter: func(v any) { set = v }}
	if binding.Name() != "Priority" || binding.Value() != 2 || binding.Error() != "bad" {
		t.Fatalf("bad binding")
	}
	binding.Set(3)
	if set != 3 {
		t.Fatalf("set=%v", set)
	}
	input := Input(binding)
	if input.Props()["name"] != "Priority" || input.Props()["value"] != 2 || input.Props()["aria-invalid"] != "true" || input.Props()["aria-describedby"] != "Priority-error" {
		t.Fatalf("binding props=%#v", input.Props())
	}
	input.Events()["onChange"](Event{Value: 4})
	if set != 4 {
		t.Fatalf("onChange set=%v", set)
	}
	errText := FieldError(binding)
	if errText.Tag() != "span" || errText.Props()["id"] != "Priority-error" || errText.Props()["role"] != "alert" {
		t.Fatalf("bad error text: %#v", errText)
	}
	selectEl := Select[int](binding, 1, 2, 3)
	if selectEl.Tag() != "select" || len(selectEl.Children()) != 3 {
		t.Fatalf("bad select: %#v", selectEl)
	}
	if NumberInput(binding).Props()["type"] != "number" {
		t.Fatal("NumberInput type")
	}
	if TextInput(binding).Props()["type"] != "text" || EmailInput(binding).Props()["type"] != "email" || PasswordInput(binding).Props()["type"] != "password" || DateInput(binding).Props()["type"] != "date" {
		t.Fatal("typed input types")
	}
	if Checkbox(SimpleField{N: "Done", V: true}).Props()["checked"] != true {
		t.Fatal("checkbox checked")
	}
	if Radio(SimpleField{N: "Priority", V: 2}, 2).Props()["checked"] != true {
		t.Fatal("radio checked")
	}
}

func TestRenderReturnsTreeForRuntimeAdapter(t *testing.T) {
	e := Div(H1(Text("Hi")))
	if got := Render(e, nil).(Element); got.Tag() != "div" {
		t.Fatalf("Render=%#v", got)
	}
	rt := &recordRuntime{}
	node := Render(Button(OnClick(func() {}), Text("go")), MountTarget{Runtime: rt, Container: "root"}).(recordNode)
	if rt.container != "root" || rt.element == nil || node.Tag != "button" || len(node.Children) != 1 || node.Props["onClick"] == nil {
		t.Fatalf("bad runtime render node=%#v rt=%#v", node, rt)
	}
	if raw := Render(Raw("x"), rt).(recordNode); raw.Raw != "x" {
		t.Fatalf("raw=%#v", raw)
	}
}

type recordNode struct {
	Tag      string
	Text     string
	Props    map[string]any
	Children []recordNode
	Raw      any
}

type recordRuntime struct {
	container any
	element   any
}

func (r *recordRuntime) CreateElement(tag string, props map[string]any, children ...any) any {
	n := recordNode{Tag: tag, Props: props}
	for _, child := range children {
		n.Children = append(n.Children, child.(recordNode))
	}
	return n
}
func (r *recordRuntime) CreateFragment(children ...any) any {
	n := recordNode{}
	for _, child := range children {
		n.Children = append(n.Children, child.(recordNode))
	}
	return n
}
func (r *recordRuntime) CreateText(text string) any { return recordNode{Text: text} }
func (r *recordRuntime) UseRaw(value any) any       { return recordNode{Raw: value} }
func (r *recordRuntime) Mount(container any, element any) {
	r.container = container
	r.element = element
}

type recordReactiveTextNode struct{ Text string }

type recordReactiveTextRuntime struct {
	node     *recordReactiveTextNode
	textRuns int
	dispose  reactive.DisposeFunc
}

func (r *recordReactiveTextRuntime) CreateElement(string, map[string]any, ...any) any {
	panic("CreateElement should not be called")
}
func (r *recordReactiveTextRuntime) CreateFragment(...any) any {
	panic("CreateFragment should not be called")
}
func (r *recordReactiveTextRuntime) CreateText(string) any { panic("CreateText should not be called") }
func (r *recordReactiveTextRuntime) UseRaw(any) any        { panic("UseRaw should not be called") }
func (r *recordReactiveTextRuntime) Mount(any, any)        {}
func (r *recordReactiveTextRuntime) CreateReactiveText(fn func() string) any {
	r.node = &recordReactiveTextNode{}
	r.dispose = reactive.Effect(func() {
		r.textRuns++
		r.node.Text = fn()
	})
	return r.node
}
