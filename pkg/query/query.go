package query

import (
	"context"
	"sync"
	"time"
)

type options struct {
	stale          time.Duration
	cache          time.Duration
	refetchOnFocus bool
}

type Opt func(*options)

func StaleTime(d time.Duration) Opt { return func(o *options) { o.stale = d } }
func CacheTime(d time.Duration) Opt { return func(o *options) { o.cache = d } }
func RefetchOnFocus(enabled bool) Opt {
	return func(o *options) { o.refetchOnFocus = enabled }
}

type QueryResult[T any] struct {
	key        Key
	normalized string
	fetch      func(context.Context) (T, error)
	opts       options
	released   bool
	releaseMu  sync.Mutex
}

func (q *QueryResult[T]) Data() T {
	var zero T
	cache.RLock()
	defer cache.RUnlock()
	if e := cache.m[q.normalized]; e != nil && e.hasData {
		if data, ok := e.data.(T); ok {
			return data
		}
	}
	return zero
}

func (q *QueryResult[T]) Err() error {
	cache.RLock()
	defer cache.RUnlock()
	if e := cache.m[q.normalized]; e != nil {
		return e.err
	}
	return nil
}

func (q *QueryResult[T]) Loading() bool {
	cache.RLock()
	defer cache.RUnlock()
	if e := cache.m[q.normalized]; e != nil {
		return e.fetching && !e.hasData
	}
	return false
}

func (q *QueryResult[T]) Fetching() bool {
	cache.RLock()
	defer cache.RUnlock()
	if e := cache.m[q.normalized]; e != nil {
		return e.fetching
	}
	return false
}

func (q *QueryResult[T]) Refetch()    { startFetch(q.normalized, true) }
func (q *QueryResult[T]) Invalidate() { Invalidate(q.key) }

// Release marks this query result as unmounted. When the last subscriber for a
// key is released, the entry is evicted after CacheTime unless it is used again.
func (q *QueryResult[T]) Release() {
	q.releaseMu.Lock()
	defer q.releaseMu.Unlock()
	if q.released {
		return
	}
	q.released = true
	cache.Lock()
	e := cache.m[q.normalized]
	if e == nil {
		cache.Unlock()
		return
	}
	if e.subscribers > 0 {
		e.subscribers--
	}
	if e.subscribers == 0 {
		e.evictSeq++
		seq := e.evictSeq
		cacheTime := e.cacheTime
		normalized := q.normalized
		cache.Unlock()
		afterFunc(cacheTime, func() {
			cache.Lock()
			defer cache.Unlock()
			current := cache.m[normalized]
			if current != nil && current.subscribers == 0 && current.evictSeq == seq {
				delete(cache.m, normalized)
			}
		})
		return
	}
	cache.Unlock()
}

func UseQuery[T any](key Key, fetch func(context.Context) (T, error), opts ...Opt) *QueryResult[T] {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	n := normalize(key)
	qr := &QueryResult[T]{key: append(Key(nil), key...), normalized: n, fetch: fetch, opts: o}
	wrapped := func() (any, error) { return fetch(context.Background()) }
	cache.Lock()
	e := cache.m[n]
	if e == nil {
		e = &entry{key: append(Key(nil), key...), stale: true}
		cache.m[n] = e
	}
	e.subscribers++
	e.fetch = wrapped
	e.staleTime = o.stale
	e.cacheTime = o.cache
	e.evictSeq++
	shouldFetch := !e.fetching && (!e.hasData || e.stale || now().Sub(e.fetched) > o.stale)
	cache.Unlock()
	if shouldFetch {
		startFetch(n, false)
	}
	if o.refetchOnFocus {
		registerFocusKey(n)
	}
	return qr
}

func defaultOptions() options {
	return options{stale: time.Minute, cache: 5 * time.Minute}
}

func startFetch(normalized string, force bool) {
	cache.Lock()
	e := cache.m[normalized]
	if e == nil || e.fetch == nil || (e.fetching && !force) {
		cache.Unlock()
		return
	}
	e.fetching = true
	e.fetchSeq++
	seq := e.fetchSeq
	fetch := e.fetch
	cache.Unlock()

	background(func() {
		data, err := fetch()
		cache.Lock()
		defer cache.Unlock()
		current := cache.m[normalized]
		if current == nil || current.fetchSeq != seq {
			return
		}
		current.fetching = false
		current.err = err
		if err == nil {
			current.data = data
			current.hasData = true
			current.stale = false
			current.fetched = now()
		}
	})
}

func refetchActive(normalized string, onlyStale bool) {
	cache.RLock()
	e := cache.m[normalized]
	if e == nil || e.subscribers == 0 {
		cache.RUnlock()
		return
	}
	stale := e.stale || now().Sub(e.fetched) > e.staleTime
	cache.RUnlock()
	if !onlyStale || stale {
		startFetch(normalized, false)
	}
}
