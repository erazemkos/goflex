package query

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Key identifies a cached query. Prefer serializable primitive parts, for
// example Key{"todos"} or Key{"todo", id}.
type Key []any

type entry struct {
	key         Key
	data        any
	err         error
	fetched     time.Time
	stale       bool
	fetching    bool
	hasData     bool
	subscribers int
	fetch       func() (any, error)
	staleTime   time.Duration
	cacheTime   time.Duration
	fetchSeq    uint64
	evictSeq    uint64
}

type cacheState struct {
	sync.RWMutex
	m map[string]*entry
}

var cache = cacheState{m: map[string]*entry{}}

var (
	now        = time.Now
	afterFunc  = time.AfterFunc
	background = func(fn func()) { go fn() }
)

func normalize(k Key) string {
	parts := make([]string, len(k))
	for i, v := range k {
		b, err := json.Marshal(v)
		if err != nil {
			b = []byte(fmt.Sprint(v))
		}
		parts[i] = string(b)
	}
	return strings.Join(parts, "/")
}

// SetData writes a value to the cache. If value is a function, it is invoked
// as an updater with the previous cached value and its first return value is
// stored. This supports typed optimistic updates such as
// func(old []Todo) []Todo.
func SetData(key Key, value any) {
	setData(normalize(key), append(Key(nil), key...), value, false)
}

func setData(normalized string, key Key, value any, preserveStale bool) {
	cache.Lock()
	defer cache.Unlock()
	e := cache.m[normalized]
	if e == nil {
		e = &entry{key: key}
		cache.m[normalized] = e
	}
	e.data = applyUpdater(e.data, value)
	e.err = nil
	e.hasData = true
	if !preserveStale {
		e.stale = false
	}
	e.fetched = now()
}

func applyUpdater(old, value any) any {
	if value == nil {
		return nil
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Func || v.Type().NumIn() != 1 || v.Type().NumOut() != 1 {
		return value
	}
	argType := v.Type().In(0)
	var arg reflect.Value
	if old == nil {
		arg = reflect.Zero(argType)
	} else {
		oldValue := reflect.ValueOf(old)
		switch {
		case oldValue.Type().AssignableTo(argType):
			arg = oldValue
		case oldValue.Type().ConvertibleTo(argType):
			arg = oldValue.Convert(argType)
		default:
			arg = reflect.Zero(argType)
		}
	}
	return v.Call([]reflect.Value{arg})[0].Interface()
}

func snapshot() map[string]entry {
	cache.RLock()
	defer cache.RUnlock()
	out := make(map[string]entry, len(cache.m))
	for k, e := range cache.m {
		out[k] = *e
	}
	return out
}

func restore(snap map[string]entry) {
	cache.Lock()
	defer cache.Unlock()
	cache.m = make(map[string]*entry, len(snap))
	for k, e := range snap {
		copy := e
		cache.m[k] = &copy
	}
}

// Snapshot returns a shallow copy of cache entries for tests and dev tools.
func Snapshot() map[string]any {
	cache.RLock()
	defer cache.RUnlock()
	out := make(map[string]any, len(cache.m))
	for k, e := range cache.m {
		out[k] = map[string]any{
			"key":         append(Key(nil), e.key...),
			"data":        e.data,
			"err":         e.err,
			"stale":       e.stale,
			"fetching":    e.fetching,
			"hasData":     e.hasData,
			"subscribers": e.subscribers,
		}
	}
	return out
}

// Clear removes all cached data. It is mainly useful in tests and during app
// teardown.
func Clear() {
	cache.Lock()
	defer cache.Unlock()
	cache.m = map[string]*entry{}
}
