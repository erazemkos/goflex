package router

import (
	"reflect"
	"testing"

	"github.com/erazemkos/goflex/pkg/ui"
)

func TestExactRouteMatches(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		params  map[string]string
		match   bool
	}{
		{"/", "/", map[string]string{}, true},
		{"/todos", "/todos", map[string]string{}, true},
		{"/todos/:id", "/todos/5", map[string]string{"id": "5"}, true},
		{"/todos/:id", "/todos/", nil, false},
		{"/settings/*", "/settings/account/email", map[string]string{"*": "account/email"}, true},
		{"/todos/:id", "/todos/a%2Fb%20c", map[string]string{"id": "a/b c"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+" "+tc.path, func(t *testing.T) {
			params, ok := MatchPattern(tc.pattern, tc.path)
			if ok != tc.match {
				t.Fatalf("match=%v want %v", ok, tc.match)
			}
			if ok && !reflect.DeepEqual(params, tc.params) {
				t.Fatalf("params=%v want %v", params, tc.params)
			}
		})
	}
}

func TestRoutePrecedence(t *testing.T) {
	r := New()
	r.Route("/users/:id", func() ui.Element { return ui.Text("id") })
	r.Route("/users/me", func() ui.Element { return ui.Text("me") })
	r.Navigate("/users/me")
	if got := r.Outlet().TextValue(); got != "me" {
		t.Fatalf("got=%q", got)
	}
}

func TestNotFoundFallback(t *testing.T) {
	r := New()
	r.Route("/", func() ui.Element { return ui.Text("home") })
	r.NotFound(func() ui.Element { return ui.Text("nf") })
	r.Navigate("/missing")
	if got := r.Outlet().TextValue(); got != "nf" {
		t.Fatalf("got=%q", got)
	}
}

func TestNavigatePushesHistory(t *testing.T) {
	h := NewMemoryHistory()
	r := New(WithHistory(h))
	r.Navigate("/todos", State("s"))
	r.Navigate("/other", Replace())
	if len(h.Calls) != 2 {
		t.Fatalf("calls=%#v", h.Calls)
	}
	if h.Calls[0].Kind != "pushState" || h.Calls[0].Path != "/todos" || h.Calls[0].State != "s" {
		t.Fatalf("push=%#v", h.Calls[0])
	}
	if h.Calls[1].Kind != "replaceState" || h.Calls[1].Path != "/other" {
		t.Fatalf("replace=%#v", h.Calls[1])
	}
}

func TestBackForwardWorks(t *testing.T) {
	h := NewMemoryHistory()
	r := New(WithHistory(h))
	r.Navigate("/a")
	r.Navigate("/b")
	r.Navigate("/c")
	h.Back()
	if got := UseLocation().Path; got != "/b" {
		t.Fatalf("back location=%q", got)
	}
	h.Forward()
	if got := UseLocation().Path; got != "/c" {
		t.Fatalf("forward location=%q", got)
	}
}

func TestUseParamsReturnsCapturedParams(t *testing.T) {
	r := New()
	r.Route("/todos/:id", func() ui.Element { return ui.Text(UseParams()["id"]) })
	r.Navigate("/todos/42")
	if got := r.Outlet().TextValue(); got != "42" {
		t.Fatalf("got=%q params=%v", got, UseParams())
	}
	params := UseParams()
	params["id"] = "mutated"
	if got := UseParams()["id"]; got != "42" {
		t.Fatalf("params were not copied: %q", got)
	}
}

func TestLinkClickBehavior(t *testing.T) {
	r := New()
	link := Link("/x", ui.Text("x"))
	events := link.Events()
	click := events["onClick"]
	if click == nil {
		t.Fatal("missing onClick")
	}
	ev := &fakeClick{}
	click(ui.Event{Value: ev})
	if !ev.Prevented || UseLocation().Path != "/x" {
		t.Fatalf("ordinary click prevented=%v loc=%s", ev.Prevented, UseLocation().Path)
	}
	_ = r

	for _, ev := range []*fakeClick{{CtrlKey: true}, {MetaKey: true}, {ShiftKey: true}, {Button: 1}} {
		r.Navigate("/")
		click(ui.Event{Value: ev})
		if ev.Prevented || UseLocation().Path != "/" {
			t.Fatalf("modified click=%#v loc=%s", ev, UseLocation().Path)
		}
	}
}

func TestHashHistoryMode(t *testing.T) {
	h := NewMemoryHistory(HistoryHash)
	r := New(WithHistory(h), WithHistoryMode(HistoryHash))
	r.Route("/todos", func() ui.Element { return ui.Text("todos") })
	r.Navigate("/todos")
	if h.Location() != "#/todos" {
		t.Fatalf("hash=%q", h.Location())
	}
	if got := r.Outlet().TextValue(); got != "todos" {
		t.Fatalf("got=%q", got)
	}
}

func TestNestedRoutes(t *testing.T) {
	r := New()
	r.Route("/app", func() ui.Element {
		return ui.Div(ui.Text("layout"), Outlet())
	}, Child("/dashboard", func() ui.Element { return ui.H1(ui.Text("dashboard")) }), Child("/profile", func() ui.Element { return ui.H1(ui.Text("profile")) }))
	r.Navigate("/app/dashboard")
	root := r.Outlet()
	if root.Tag() != "div" || len(root.Children()) != 2 {
		t.Fatalf("root=%#v", root)
	}
	if root.Children()[1].Tag() != "h1" || root.Children()[1].Children()[0].TextValue() != "dashboard" {
		t.Fatalf("child=%#v", root.Children()[1])
	}
}

func TestTrailingSlashNormalization(t *testing.T) {
	for _, path := range []string{"/foo", "/foo/"} {
		r := New()
		r.Route("/foo/", func() ui.Element { return ui.Text("foo") })
		r.Navigate(path)
		if got := r.Outlet().TextValue(); got != "foo" {
			t.Fatalf("%s got %q", path, got)
		}
	}
}

func TestUseNavigateAndRoot(t *testing.T) {
	r := New()
	r.Route("/", func() ui.Element { return ui.Text("home") })
	r.Route("/x", func() ui.Element { return ui.Text("x") })
	UseNavigate()("/x", Scroll(Preserve))
	if UseLocation().Path != "/x" || r.Root().TextValue() != "x" {
		t.Fatalf("loc=%#v root=%#v", UseLocation(), r.Root())
	}
}

func TestMemoryHistoryReplaceOnEmptyAndInitialHash(t *testing.T) {
	h := &MemoryHistory{mode: HistoryHash}
	h.Replace("/x", 1)
	if h.Location() != "#/x" || h.Calls[0].Kind != "replaceState" {
		t.Fatalf("history=%#v", h)
	}
	r := New(WithHistory(h), WithHistoryMode(HistoryHash))
	if UseLocation().Path != "/x" || r.currentState != nil {
		t.Fatalf("location=%#v", UseLocation())
	}
}

type fakeClick struct {
	Button    int
	CtrlKey   bool
	MetaKey   bool
	ShiftKey  bool
	Prevented bool
}

func (f *fakeClick) PreventDefault() { f.Prevented = true }
