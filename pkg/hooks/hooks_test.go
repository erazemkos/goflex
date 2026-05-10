package hooks

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/goflex/goflex/pkg/ui"
)

func TestUseStateBasic(t *testing.T) {
	Reset()
	BeginRender("counter")
	s := UseState(0)
	if got := s.Get(); got != 0 {
		t.Fatalf("initial=%d", got)
	}
	EndRender()
	s.Set(5)
	if !RenderRequested("counter") {
		t.Fatal("setter did not request render")
	}

	BeginRender("counter")
	next := UseState(0)
	if got := next.Get(); got != 5 {
		t.Fatalf("next render state=%d", got)
	}
	EndRender()
}

func TestUseStateUpdateRecordsFunction(t *testing.T) {
	Reset()
	BeginRender("counter")
	s := UseState(1)
	EndRender()
	s.Update(func(v int) int { return v + 1 })
	if got := s.Get(); got != 2 {
		t.Fatalf("updated=%d", got)
	}
	calls := SetCalls("counter")
	if len(calls) != 1 {
		t.Fatalf("calls=%d", len(calls))
	}
	if _, ok := calls[0].(func(int) int); !ok {
		t.Fatalf("setter call type=%T", calls[0])
	}
}

func TestUseEffectRunsWithDepsAndCleanup(t *testing.T) {
	Reset()
	var order []string

	BeginRender("effect")
	UseEffect(func() func() {
		order = append(order, "run:1")
		return func() { order = append(order, "cleanup:1") }
	}, 1)
	EndRender()

	BeginRender("effect")
	UseEffect(func() func() {
		order = append(order, "run:same")
		return nil
	}, 1)
	EndRender()

	BeginRender("effect")
	UseEffect(func() func() {
		order = append(order, "run:2")
		return func() { order = append(order, "cleanup:2") }
	}, 2)
	EndRender()
	Unmount("effect")

	want := strings.Join([]string{"run:1", "cleanup:1", "run:2", "cleanup:2"}, ",")
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("order=%s want %s", got, want)
	}
}

func TestUseEffectNilCleanupAndUnmount(t *testing.T) {
	Reset()
	called := 0
	BeginRender("nil-cleanup")
	UseEffect(func() func() { called++; return nil }, "x")
	EndRender()
	Unmount("nil-cleanup")
	if called != 1 {
		t.Fatalf("called=%d", called)
	}
}

func TestUseMemo(t *testing.T) {
	Reset()
	calls := 0
	BeginRender("memo")
	v := UseMemo(func() int { calls++; return 10 }, "a")
	EndRender()
	BeginRender("memo")
	v2 := UseMemo(func() int { calls++; return 20 }, "a")
	EndRender()
	BeginRender("memo")
	v3 := UseMemo(func() int { calls++; return 30 }, "b")
	EndRender()
	if v != 10 || v2 != 10 || v3 != 30 || calls != 2 {
		t.Fatalf("v=%d v2=%d v3=%d calls=%d", v, v2, v3, calls)
	}
}

func TestUseRefDoesNotRequestRender(t *testing.T) {
	Reset()
	BeginRender("ref")
	r := UseRef(42)
	EndRender()
	if r.Current != 42 {
		t.Fatalf("current=%d", r.Current)
	}
	r.Current = 100
	if RenderRequested("ref") {
		t.Fatal("ref mutation requested render")
	}
	BeginRender("ref")
	r2 := UseRef(0)
	EndRender()
	if r2 != r || r2.Current != 100 {
		t.Fatalf("ref not stable: %#v %#v", r, r2)
	}
}

func TestUseReducer(t *testing.T) {
	Reset()
	reducer := func(s int, a string) int {
		if a == "inc" {
			return s + 1
		}
		return s
	}
	BeginRender("reducer")
	state, dispatch := UseReducer(reducer, 0)
	EndRender()
	if state != 0 {
		t.Fatalf("state=%d", state)
	}
	dispatch("inc")
	BeginRender("reducer")
	state, _ = UseReducer(reducer, 0)
	EndRender()
	if state != 1 {
		t.Fatalf("state=%d", state)
	}
}

func TestContext(t *testing.T) {
	ctx := CreateContext("default")
	if got := UseContext(ctx); got != "default" {
		t.Fatalf("default=%q", got)
	}
	ctx.Provider("hello", ui.Text("child"))
	if got := UseContext(ctx); got != "hello" {
		t.Fatalf("provider=%q", got)
	}
	ctx.Clear()
	if got := UseContext(ctx); got != "default" {
		t.Fatalf("cleared=%q", got)
	}
	got := ctx.WithProvider("scoped", func() ui.Element {
		return ui.Text(UseContext(ctx))
	})
	if got.TextValue() != "scoped" || UseContext(ctx) != "default" {
		t.Fatalf("scoped=%q after=%q", got.TextValue(), UseContext(ctx))
	}
}

func TestRulesOfHooksEnforcementDevMode(t *testing.T) {
	Reset()
	old := ui.DevMode
	ui.DevMode = true
	defer func() { ui.DevMode = old }()
	BeginRender("bad")
	UseState(1)
	EndRender()
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "hooks must be called in the same order") {
			t.Fatalf("panic=%v", r)
		}
	}()
	BeginRender("bad")
	EndRender()
}

func TestRulesOfHooksNoPanicProductionMode(t *testing.T) {
	Reset()
	old := ui.DevMode
	ui.DevMode = false
	defer func() { ui.DevMode = old }()
	BeginRender("prod")
	UseState(1)
	EndRender()
	BeginRender("prod")
	EndRender()
}

func TestHookKindOrderingEnforcement(t *testing.T) {
	Reset()
	old := ui.DevMode
	ui.DevMode = true
	defer func() { ui.DevMode = old }()
	BeginRender("kind")
	UseState(1)
	EndRender()
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "hooks must be called in the same order") {
			t.Fatalf("panic=%v", r)
		}
	}()
	BeginRender("kind")
	UseMemo(func() int { return 1 })
}

func TestDependencyWarnings(t *testing.T) {
	Reset()
	old := ui.DevMode
	ui.DevMode = true
	defer func() { ui.DevMode = old }()
	var buf bytes.Buffer
	SetWarningWriter(&buf)
	defer SetWarningWriter(nil)
	BeginRender("deps")
	UseMemo(func() int { return 1 }, []int{1})
	UseEffect(func() func() { return nil }, map[string]int{"x": 1})
	EndRender()
	warnings := Warnings("deps")
	if len(warnings) != 2 || !strings.Contains(buf.String(), "not comparable") {
		t.Fatalf("warnings=%v buf=%s", warnings, buf.String())
	}
}

func TestUseCallback(t *testing.T) {
	Reset()
	fn1 := func() string { return "one" }
	fn2 := func() string { return "two" }
	BeginRender("callback")
	got1 := UseCallback(fn1, 1).(func() string)
	EndRender()
	BeginRender("callback")
	got2 := UseCallback(fn2, 1).(func() string)
	EndRender()
	BeginRender("callback")
	got3 := UseCallback(fn2, 2).(func() string)
	EndRender()
	if got1() != "one" || got2() != "one" || got3() != "two" {
		t.Fatalf("callbacks returned %q %q %q", got1(), got2(), got3())
	}
}

func TestUIComponentIntegrationBindsFiber(t *testing.T) {
	Reset()
	counter := ui.Component("Counter", func(p ui.Props) ui.Element {
		s := UseState(0)
		return ui.Button(ui.OnClick(func() { s.Update(func(v int) int { return v + 1 }) }), ui.Textf("count=%d", s.Get()))
	})
	first := counter(nil)
	if first.Children()[0].Children()[0].TextValue() != "count=0" {
		t.Fatalf("first=%#v", first)
	}
	first.Children()[0].Events()["onClick"](ui.Event{})
	second := counter(nil)
	if second.Children()[0].Children()[0].TextValue() != "count=1" {
		t.Fatalf("second=%#v", second)
	}
}

func TestTypedHookHelpersCompileForCommonTypes(t *testing.T) {
	Reset()
	type user struct{ Name string }
	BeginRender("types")
	_ = UseState(1)
	_ = UseState("x")
	_ = UseState(true)
	_ = UseState(struct{ Name string }{"n"})
	_ = UseState([]int{1})
	_ = UseState(map[string]int{"x": 1})
	_ = UseState(&user{Name: "u"})
	_ = UseState[any](errors.New("x"))
	_ = UseMemo(func() []int { return []int{1} }, "stable")
	_ = UseRef(map[string]int{})
	_, _ = UseReducer(func(s user, a string) user { s.Name = a; return s }, user{})
	EndRender()
}
