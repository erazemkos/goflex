package query

import "strings"

// Invalidate marks all cache entries whose key starts with keyPrefix stale. If
// a matching query is currently mounted, it refetches in the background.
func Invalidate(keyPrefix Key) {
	prefix := normalize(keyPrefix)
	var active []string
	cache.Lock()
	for k, e := range cache.m {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			e.stale = true
			if e.subscribers > 0 && e.fetch != nil {
				active = append(active, k)
			}
		}
	}
	cache.Unlock()
	for _, k := range active {
		startFetch(k, false)
	}
}
