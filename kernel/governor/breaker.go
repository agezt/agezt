// SPDX-License-Identifier: MIT

package governor

import (
	"sync"
	"time"
)

// DefaultBreakerThreshold is the number of consecutive fall-back failures that
// trips a provider's circuit open.
const DefaultBreakerThreshold = 5

// DefaultBreakerCooldown is how long a tripped provider stays open before a
// single half-open probe is allowed through.
const DefaultBreakerCooldown = 30 * time.Second

// breaker is a per-provider circuit breaker (M997). A provider that fails
// (fall-back-worthy errors) BreakerThreshold times in a row is skipped for the
// cooldown window, after which one probe is allowed; success closes it, another
// failure reopens it. This stops the Governor from burning latency on a provider
// that is down on every request when other providers in the chain can serve.
type breaker struct {
	mu        sync.Mutex
	states    map[string]*pbState
	threshold int
	cooldown  time.Duration
	now       func() time.Time
}

type pbState struct {
	failures  int
	openUntil time.Time // zero = closed
}

func newBreaker(threshold int, cooldown time.Duration, now func() time.Time) *breaker {
	if now == nil {
		now = time.Now
	}
	return &breaker{
		states:    map[string]*pbState{},
		threshold: threshold,
		cooldown:  cooldown,
		now:       now,
	}
}

// enabled reports whether the breaker is active. A non-positive threshold
// disables it entirely (every call is allowed, success/failure are no-ops).
func (b *breaker) enabled() bool { return b != nil && b.threshold > 0 }

// allow reports whether a call to the named provider may proceed. It is a
// read-only check: closed breakers always allow; an open one allows once the
// cooldown has elapsed (half-open). The next success closes it; the next failure
// reopens it.
func (b *breaker) allow(name string) bool {
	if !b.enabled() {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.states[name]
	if st == nil || st.openUntil.IsZero() {
		return true
	}
	return !b.now().Before(st.openUntil) // open until cooldown elapses
}

// success resets the named provider's breaker to fully closed. It returns true
// when this call recovered a previously-open/half-open circuit (a state
// transition worth surfacing).
func (b *breaker) success(name string) (recovered bool) {
	if !b.enabled() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.states[name]
	if st == nil {
		return false
	}
	wasOpen := !st.openUntil.IsZero()
	st.failures = 0
	st.openUntil = time.Time{}
	return wasOpen
}

// failure records a fall-back-worthy failure. Consecutive failures trip the
// circuit at threshold; a failure while half-open (failures already at
// threshold) reopens it for another cooldown. It returns true when this call
// (re)opened the circuit, so the caller can surface the transition.
func (b *breaker) failure(name string) (tripped bool) {
	if !b.enabled() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.states[name]
	if st == nil {
		st = &pbState{}
		b.states[name] = st
	}
	wasOpen := !st.openUntil.IsZero() && b.now().Before(st.openUntil)
	if st.failures < b.threshold {
		st.failures++
	}
	if st.failures >= b.threshold {
		st.openUntil = b.now().Add(b.cooldown)
	}
	nowOpen := !st.openUntil.IsZero() && b.now().Before(st.openUntil)
	return nowOpen && !wasOpen
}

// state returns "closed", "open" or "half-open" for diagnostics.
func (b *breaker) state(name string) string {
	if !b.enabled() {
		return "disabled"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.states[name]
	if st == nil || st.openUntil.IsZero() {
		return "closed"
	}
	if b.now().Before(st.openUntil) {
		return "open"
	}
	return "half-open"
}
