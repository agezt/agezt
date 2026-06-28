// SPDX-License-Identifier: MIT

// Package streamlimit caps the number of concurrent long-lived streams (SSE
// connections) a single client key may hold open at once. It is a resource
// guardrail in the same spirit as the HTTP body caps and slow-loris timeouts
// the API surfaces already apply (V-009): an authenticated token holder, a
// leaked token, or a buggy/compromised local client must not be able to exhaust
// file descriptors and goroutines by opening an unbounded number of streams.
//
// It is deliberately generous — a real browser opens a handful of EventSource
// connections and an SDK a few more, so the default never trips for legitimate
// use; it only bounds the pathological case. A non-positive Max disables the
// limiter entirely (unlimited), so the guardrail is opt-out, consistent with the
// project's default-allow posture.
package streamlimit

import "sync"

// Limiter bounds concurrent stream holders per key. The zero value is unusable;
// construct with New. A nil *Limiter is valid and unlimited (Acquire always
// succeeds), so callers can hold an optional limiter without nil-checks.
type Limiter struct {
	max int

	mu     sync.Mutex
	active map[string]int
}

// New returns a Limiter allowing at most max concurrent streams per key. A
// max <= 0 yields an always-allow (unlimited) limiter.
func New(max int) *Limiter {
	return &Limiter{max: max, active: make(map[string]int)}
}

// Acquire reserves one stream slot for key. It returns a release func and true
// when the key is under its cap; when the cap is already reached it returns a
// no-op release and false (the caller should refuse the stream, e.g. 429).
//
// release is idempotent — calling it more than once is safe — so the typical
// `release, ok := lim.Acquire(k); if !ok { ... }; defer release()` pattern is
// correct even if the handler also calls release explicitly. A nil limiter or a
// non-positive Max always returns (no-op, true).
func (l *Limiter) Acquire(key string) (release func(), ok bool) {
	noop := func() {}
	if l == nil || l.max <= 0 {
		return noop, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[key] >= l.max {
		return noop, false
	}
	l.active[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.active[key] <= 1 {
				delete(l.active, key) // keep the map from growing unbounded with idle keys
			} else {
				l.active[key]--
			}
		})
	}, true
}
