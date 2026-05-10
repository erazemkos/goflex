package router

import (
	"net/url"
	"strings"
	"sync"

	"github.com/goflex/goflex/pkg/ui"
)

// Component is a frontend route component.
type Component func() ui.Element

// RouteOpt configures child routes.
type RouteOpt struct {
	pattern string
	comp    Component
}

type route struct {
	pattern  string
	comp     Component
	children []route
}

// Location describes the current client-side URL.
type Location struct {
	Path  string
	State any
}

// HistoryMode controls how navigations are represented in the browser URL.
type HistoryMode int

const (
	// HistoryBrowser uses normal paths with pushState.
	HistoryBrowser HistoryMode = iota
	// HistoryHash stores paths in location.hash as #/path.
	HistoryHash
)

// History is the pluggable navigation adapter used by Router.
type History interface {
	Push(path string, state any)
	Replace(path string, state any)
	Location() string
	Listen(func(path string, state any))
}

// Router maps paths to components and owns current navigation state.
type Router struct {
	routes   []route
	notFound Component
	history  History
	mode     HistoryMode

	currentPath  string
	currentState any
	params       map[string]string

	chain       []Component
	outletIndex int
}

// Option configures a Router.
type Option func(*Router)

// WithHistory injects a custom history adapter for tests or browser runtimes.
func WithHistory(h History) Option { return func(r *Router) { r.history = h } }

// WithHistoryMode selects browser or hash history mode.
func WithHistoryMode(mode HistoryMode) Option { return func(r *Router) { r.mode = mode } }

var currentMu sync.RWMutex
var current *Router

// New creates a Router with in-memory browser-style history by default.
func New(opts ...Option) *Router {
	r := &Router{mode: HistoryBrowser, params: map[string]string{}, currentPath: "/"}
	for _, opt := range opts {
		opt(r)
	}
	if r.history == nil {
		r.history = NewMemoryHistory(r.mode)
	}
	r.currentPath = parseHistoryLocation(r.history.Location(), r.mode)
	r.history.Listen(func(path string, state any) {
		r.setLocation(path, state)
	})
	setCurrent(r)
	return r
}

func setCurrent(r *Router) {
	currentMu.Lock()
	current = r
	currentMu.Unlock()
}

func getCurrent() *Router {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}

// Child defines a nested child route.
func Child(pattern string, c Component) RouteOpt { return RouteOpt{pattern: pattern, comp: c} }

// Route registers a route pattern and optional one-level children.
func (r *Router) Route(pattern string, c Component, children ...RouteOpt) {
	base := norm(pattern)
	rt := route{pattern: base, comp: c}
	for _, ch := range children {
		rt.children = append(rt.children, route{pattern: joinPattern(base, ch.pattern), comp: ch.comp})
	}
	r.routes = append(r.routes, rt)
}

// NotFound registers a fallback route for unmatched paths.
func (r *Router) NotFound(c Component) { r.notFound = c }

// Root renders the currently matched route tree.
func (r *Router) Root() ui.Element { return r.Outlet() }

// Outlet renders the next matched component. Inside a layout this renders the matched child.
func (r *Router) Outlet() ui.Element {
	setCurrent(r)
	chain, params := r.matchChain(r.currentPath)
	if len(chain) == 0 {
		r.params = map[string]string{}
		if r.notFound != nil {
			return r.notFound()
		}
		return ui.Text("not found")
	}
	r.params = params
	r.chain = chain
	r.outletIndex = 0
	return r.renderNext()
}

func (r *Router) renderNext() ui.Element {
	if r.outletIndex >= len(r.chain) {
		return ui.Fragment()
	}
	idx := r.outletIndex
	r.outletIndex++
	return r.chain[idx]()
}

// Navigate updates history and the current route.
func (r *Router) Navigate(path string, opts ...NavOpt) {
	o := navOptions{scroll: Top}
	for _, fn := range opts {
		fn(&o)
	}
	logical := norm(path)
	if o.replace {
		r.history.Replace(logical, o.state)
	} else {
		r.history.Push(logical, o.state)
	}
	r.setLocation(logical, o.state)
}

func (r *Router) setLocation(path string, state any) {
	r.currentPath = norm(path)
	r.currentState = state
	_, r.params = r.matchChain(r.currentPath)
	setCurrent(r)
}

// UseNavigate returns a programmatic navigation function.
func UseNavigate() func(string, ...NavOpt) {
	return func(p string, opts ...NavOpt) {
		if r := getCurrent(); r != nil {
			r.Navigate(p, opts...)
		}
	}
}

// UseLocation returns the current router location.
func UseLocation() Location {
	if r := getCurrent(); r != nil {
		return Location{Path: r.currentPath, State: r.currentState}
	}
	return Location{Path: "/"}
}

// UseParams returns a copy of captured route parameters.
func UseParams() map[string]string {
	out := map[string]string{}
	if r := getCurrent(); r != nil {
		for k, v := range r.params {
			out[k] = v
		}
	}
	return out
}

// Outlet renders the active nested route outlet for the current router.
func Outlet() ui.Element {
	if r := getCurrent(); r != nil {
		return r.renderNext()
	}
	return ui.Fragment()
}

type navOptions struct {
	replace bool
	state   any
	scroll  ScrollMode
}

// NavOpt configures navigation behavior.
type NavOpt func(*navOptions)

// Replace causes navigation to use replaceState.
func Replace() NavOpt { return func(o *navOptions) { o.replace = true } }

// State attaches history state to the navigation.
func State(v any) NavOpt { return func(o *navOptions) { o.state = v } }

// ScrollMode controls scroll restoration behavior.
type ScrollMode int

const (
	// Top scrolls to the top after navigation.
	Top ScrollMode = iota
	// Preserve keeps the current scroll position.
	Preserve
)

// Scroll configures scroll behavior for navigation.
func Scroll(m ScrollMode) NavOpt { return func(o *navOptions) { o.scroll = m } }

func (r *Router) matchChain(path string) ([]Component, map[string]string) {
	path = norm(path)
	bestScore := -1
	var best []Component
	bestParams := map[string]string{}
	for _, rt := range r.routes {
		if params, ok, score := matchPattern(rt.pattern, path); ok && score > bestScore {
			bestScore = score
			best = []Component{rt.comp}
			bestParams = params
		}
		for _, child := range rt.children {
			if params, ok, score := matchPattern(child.pattern, path); ok && score > bestScore {
				bestScore = score
				best = []Component{rt.comp, child.comp}
				bestParams = params
			}
		}
	}
	return best, bestParams
}

// MatchPattern exposes route matching for table-driven tests and custom tooling.
func MatchPattern(pattern, path string) (map[string]string, bool) {
	params, ok, _ := matchPattern(pattern, path)
	return params, ok
}

func matchPattern(pattern, path string) (map[string]string, bool, int) {
	pattern = norm(pattern)
	path = norm(path)
	if pattern == "/" {
		if path == "/" {
			return map[string]string{}, true, 10000
		}
		return nil, false, 0
	}
	ps := strings.Split(strings.Trim(pattern, "/"), "/")
	xs := strings.Split(strings.Trim(path, "/"), "/")
	params := map[string]string{}
	score := 0
	for i, p := range ps {
		if p == "*" {
			params["*"] = strings.Join(xs[i:], "/")
			return params, true, score + 1
		}
		if i >= len(xs) {
			return nil, false, 0
		}
		if strings.HasPrefix(p, ":") {
			v, _ := url.PathUnescape(xs[i])
			params[p[1:]] = v
			score += 10
			continue
		}
		if p != xs[i] {
			return nil, false, 0
		}
		score += 100
	}
	if len(ps) != len(xs) {
		return nil, false, 0
	}
	return params, true, score + 1000
}

func norm(p string) string {
	if p == "" {
		return "/"
	}
	p = strings.TrimPrefix(p, "#")
	if q := strings.IndexAny(p, "?#"); q >= 0 {
		p = p[:q]
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func joinPattern(base, child string) string {
	if base == "/" {
		return norm(child)
	}
	return norm(strings.TrimRight(base, "/") + "/" + strings.TrimLeft(child, "/"))
}

func parseHistoryLocation(location string, mode HistoryMode) string {
	if mode == HistoryHash {
		if idx := strings.Index(location, "#"); idx >= 0 {
			return norm(location[idx+1:])
		}
	}
	return norm(location)
}
