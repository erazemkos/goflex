package router

// MemoryHistory is a pure-Go history adapter used by tests and non-browser runtimes.
type MemoryHistory struct {
	mode      HistoryMode
	stack     []historyEntry
	idx       int
	listeners []func(path string, state any)
	Calls     []HistoryCall
}

type historyEntry struct {
	path  string
	state any
}

// HistoryCall records a pushState/replaceState-style operation.
type HistoryCall struct {
	Kind  string
	Path  string
	State any
}

// NewMemoryHistory creates an in-memory history adapter.
func NewMemoryHistory(modes ...HistoryMode) *MemoryHistory {
	mode := HistoryBrowser
	if len(modes) > 0 {
		mode = modes[0]
	}
	return &MemoryHistory{mode: mode, stack: []historyEntry{{path: "/"}}}
}

// Push records a pushState operation.
func (h *MemoryHistory) Push(path string, state any) {
	path = norm(path)
	h.stack = h.stack[:h.idx+1]
	h.stack = append(h.stack, historyEntry{path: path, state: state})
	h.idx++
	h.Calls = append(h.Calls, HistoryCall{Kind: "pushState", Path: h.external(path), State: state})
}

// Replace records a replaceState operation.
func (h *MemoryHistory) Replace(path string, state any) {
	path = norm(path)
	if len(h.stack) == 0 {
		h.stack = []historyEntry{{path: path, state: state}}
		h.idx = 0
	} else {
		h.stack[h.idx] = historyEntry{path: path, state: state}
	}
	h.Calls = append(h.Calls, HistoryCall{Kind: "replaceState", Path: h.external(path), State: state})
}

// Location returns the external browser-like location string.
func (h *MemoryHistory) Location() string {
	if len(h.stack) == 0 {
		return h.external("/")
	}
	return h.external(h.stack[h.idx].path)
}

// Listen registers a popstate/hashchange listener.
func (h *MemoryHistory) Listen(fn func(path string, state any)) {
	h.listeners = append(h.listeners, fn)
}

// Back simulates browser back.
func (h *MemoryHistory) Back() {
	if h.idx > 0 {
		h.idx--
		h.notify()
	}
}

// Forward simulates browser forward.
func (h *MemoryHistory) Forward() {
	if h.idx+1 < len(h.stack) {
		h.idx++
		h.notify()
	}
}

func (h *MemoryHistory) notify() {
	entry := h.stack[h.idx]
	for _, fn := range h.listeners {
		fn(entry.path, entry.state)
	}
}

func (h *MemoryHistory) external(path string) string {
	if h.mode == HistoryHash {
		return "#" + norm(path)
	}
	return norm(path)
}
