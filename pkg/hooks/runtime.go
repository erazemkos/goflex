package hooks

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"

	"github.com/erazemkos/goflex/pkg/ui"
)

type hookSlot struct {
	kind        string
	initialized bool
	value       any
	deps        []any
	cleanup     func()
}

type fiber struct {
	id              string
	cursor          int
	expectedHooks   int
	slots           []hookSlot
	renderRequested bool
	setCalls        []any
	warnings        []string
}

var runtimeState = struct {
	sync.Mutex
	fibers        map[string]*fiber
	current       *fiber
	warningWriter io.Writer
}{
	fibers:        map[string]*fiber{},
	warningWriter: io.Discard,
}

func init() {
	ui.SetComponentRenderHook(func(name string) func() {
		BeginRender(name)
		return EndRender
	})
}

// BeginRender starts a mock/component render cycle. An optional id scopes hook state to a component fiber.
func BeginRender(ids ...string) {
	id := "default"
	if len(ids) > 0 && ids[0] != "" {
		id = ids[0]
	}
	runtimeState.Lock()
	f := getFiberLocked(id)
	f.cursor = 0
	runtimeState.current = f
	runtimeState.Unlock()
}

// EndRender finishes a render cycle and enforces stable hook ordering in dev mode.
func EndRender() {
	runtimeState.Lock()
	f := runtimeState.current
	if f == nil {
		runtimeState.Unlock()
		return
	}
	count := f.cursor
	if ui.DevMode && f.expectedHooks >= 0 && f.expectedHooks != count {
		runtimeState.current = nil
		runtimeState.Unlock()
		panic(fmt.Sprintf("hooks must be called in the same order: previous render used %d hooks, current render used %d", f.expectedHooks, count))
	}
	f.expectedHooks = count
	runtimeState.current = nil
	runtimeState.Unlock()
}

// Unmount runs effect cleanup functions for the named fiber and drops its hook state.
func Unmount(ids ...string) {
	id := "default"
	if len(ids) > 0 && ids[0] != "" {
		id = ids[0]
	}
	runtimeState.Lock()
	f := runtimeState.fibers[id]
	delete(runtimeState.fibers, id)
	if runtimeState.current == f {
		runtimeState.current = nil
	}
	runtimeState.Unlock()
	if f == nil {
		return
	}
	for _, slot := range f.slots {
		if slot.cleanup != nil {
			slot.cleanup()
		}
	}
}

// Reset clears all mock hook state. It is intended for tests.
func Reset() {
	runtimeState.Lock()
	runtimeState.fibers = map[string]*fiber{}
	runtimeState.current = nil
	runtimeState.Unlock()
}

// RenderRequested reports whether a state setter requested a re-render for a fiber.
func RenderRequested(ids ...string) bool {
	f := fiberByOptionalID(ids...)
	if f == nil {
		return false
	}
	return f.renderRequested
}

// SetCalls returns values/functions passed to State setters for a fiber.
func SetCalls(ids ...string) []any {
	f := fiberByOptionalID(ids...)
	if f == nil {
		return nil
	}
	return append([]any(nil), f.setCalls...)
}

// Warnings returns dependency warnings emitted for a fiber.
func Warnings(ids ...string) []string {
	f := fiberByOptionalID(ids...)
	if f == nil {
		return nil
	}
	return append([]string(nil), f.warnings...)
}

// SetWarningWriter configures where development dependency warnings are written.
func SetWarningWriter(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	runtimeState.Lock()
	runtimeState.warningWriter = w
	runtimeState.Unlock()
}

func fiberByOptionalID(ids ...string) *fiber {
	id := "default"
	if len(ids) > 0 && ids[0] != "" {
		id = ids[0]
	}
	runtimeState.Lock()
	defer runtimeState.Unlock()
	return runtimeState.fibers[id]
}

func currentFiberLocked() *fiber {
	if runtimeState.current == nil {
		runtimeState.current = getFiberLocked("default")
	}
	return runtimeState.current
}

func getFiberLocked(id string) *fiber {
	f := runtimeState.fibers[id]
	if f == nil {
		f = &fiber{id: id, expectedHooks: -1}
		runtimeState.fibers[id] = f
	}
	return f
}

func claimSlot(kind string) (*fiber, int, hookSlot) {
	runtimeState.Lock()
	f := currentFiberLocked()
	idx := f.cursor
	f.cursor++
	for len(f.slots) <= idx {
		f.slots = append(f.slots, hookSlot{})
	}
	slot := f.slots[idx]
	if ui.DevMode && slot.kind != "" && slot.kind != kind {
		runtimeState.Unlock()
		panic("hooks must be called in the same order: hook kind changed")
	}
	if slot.kind == "" {
		slot.kind = kind
		f.slots[idx] = slot
	}
	runtimeState.Unlock()
	return f, idx, slot
}

func depsChanged(old []any, next []any) bool {
	if len(old) != len(next) {
		return true
	}
	for i := range old {
		if !objectIs(old[i], next[i]) {
			return true
		}
	}
	return false
}

func objectIs(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return reflect.DeepEqual(a, b)
	}
	return reflect.ValueOf(a).Interface() == reflect.ValueOf(b).Interface()
}

func copyDeps(deps []any) []any { return append([]any(nil), deps...) }

func warnNonComparableDeps(f *fiber, deps []any) {
	if !ui.DevMode {
		return
	}
	for _, dep := range deps {
		if dep == nil {
			continue
		}
		t := reflect.TypeOf(dep)
		if t.Comparable() {
			continue
		}
		msg := fmt.Sprintf("level=warn msg=\"hook dependency %s is not comparable; use UseMemo or a stable pointer\"", t.String())
		runtimeState.Lock()
		f.warnings = append(f.warnings, msg)
		w := runtimeState.warningWriter
		runtimeState.Unlock()
		_, _ = fmt.Fprintln(w, msg)
	}
}

// EnableWarningsToStderr sends development dependency warnings to stderr.
func EnableWarningsToStderr() { SetWarningWriter(os.Stderr) }
