package main

import (
	"errors"
	"sync"
	"time"
)

// cached wraps scan so that its result for a key is kept for lifetime, measured with now from the end of the scan, if
// keep accepts it. Calls for that key within that time reuse the result, and calls while the scan is still running
// wait for it. Failed scans are not kept.
func cached[K comparable, V any](
	scan func(K) (V, error), keep func(V) bool, lifetime time.Duration, now func() time.Time,
) func(K) (V, error) {
	type entry struct {
		done    chan struct{} // closed when the scan has finished
		value   V
		err     error
		expires time.Time // zero while the scan is running
	}
	var mu sync.Mutex
	entries := map[K]*entry{}

	run := func(key K, e *entry) {
		e.err = errors.New("the scan failed unexpectedly") // kept only if scan panics
		defer func() {
			kept := e.err == nil && keep(e.value)
			mu.Lock()
			if !kept {
				delete(entries, key)
			} else {
				e.expires = now().Add(lifetime)
			}
			mu.Unlock()
			close(e.done)
		}()
		e.value, e.err = scan(key)
	}

	return func(key K) (V, error) {
		mu.Lock()
		t := now()
		for k, e := range entries {
			if !e.expires.IsZero() && !t.Before(e.expires) {
				delete(entries, k)
			}
		}
		e, ok := entries[key]
		if ok {
			mu.Unlock()
			<-e.done
		} else {
			e = &entry{done: make(chan struct{})}
			entries[key] = e
			mu.Unlock()
			run(key, e)
		}
		return e.value, e.err
	}
}
