package api

import (
	"sonmi/internal/db"
	"sync"
)

type RationaleCorrelator struct {
	mu sync.Mutex
	m  map[string]string
}

func NewRationaleCorrelator() *RationaleCorrelator {
	return &RationaleCorrelator{m: make(map[string]string)}
}

func rationaleKey(relay db.Relay_t, mode db.Mode_t) string {
	return string(relay) + ":" + string(mode)
}

func (r *RationaleCorrelator) Put(relay db.Relay_t, mode db.Mode_t, rationale string) {
	if r == nil || rationale == "" {
		return
	}

	r.mu.Lock()
	r.m[rationaleKey(relay, mode)] = rationale
	r.mu.Unlock()
}

func (r *RationaleCorrelator) Take(relay db.Relay_t, mode db.Mode_t) string {
	if r == nil {
		return ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := rationaleKey(relay, mode)
	v := r.m[key]
	delete(r.m, key)

	return v
}
