package query

import (
	"sort"
	"sync"

	"github.com/erazemkos/goflex/pkg/ui"
)

var focus = struct {
	sync.Mutex
	keys map[string]struct{}
}{keys: map[string]struct{}{}}

// Provider mounts a query cache boundary. The current implementation uses the
// process-wide in-memory cache and returns the children unchanged.
func Provider(children ...ui.Element) ui.Element { return ui.Fragment(children...) }

func registerFocusKey(normalized string) {
	focus.Lock()
	defer focus.Unlock()
	focus.keys[normalized] = struct{}{}
}

// Focus simulates a browser focus/reconnect event and refetches active stale
// queries that opted into RefetchOnFocus.
func Focus() {
	focus.Lock()
	keys := make([]string, 0, len(focus.keys))
	for key := range focus.keys {
		keys = append(keys, key)
	}
	focus.Unlock()
	sort.Strings(keys)
	for _, key := range keys {
		refetchActive(key, true)
	}
}

// Inspector returns a dev-tools friendly snapshot of query state. A browser
// runtime can expose this value as window.__GOFLEX_QUERY__ when ui.DevMode is
// enabled.
func Inspector() map[string]any { return Snapshot() }
