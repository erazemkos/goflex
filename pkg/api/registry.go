package api

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type EndpointInfo struct {
	Method   string
	Path     string
	Endpoint any
}

type registry struct {
	mu    sync.RWMutex
	items map[string]EndpointInfo
}

var Registry = &registry{items: map[string]EndpointInfo{}}

func (r *registry) Register(method, path string, ep any) {
	method = strings.ToUpper(method)
	k := method + " " + path
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[k]; ok {
		panic(fmt.Sprintf("duplicate endpoint %s", k))
	}
	r.items[k] = EndpointInfo{Method: method, Path: path, Endpoint: ep}
}

func (r *registry) All() []EndpointInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]EndpointInfo, 0, len(r.items))
	for _, v := range r.items {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Method == out[j].Method {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

func (r *registry) ResetForTest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = map[string]EndpointInfo{}
}
