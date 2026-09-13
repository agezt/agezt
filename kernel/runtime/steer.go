// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/intervention"
)


// runControl is the per-run steering surface. It implements agent.Steerer
// (Wait + Drain) for the loop and exposes Pause/Resume/Step/Inject for the
// operator. The zero value is not usable — construct with newRunControl.
type runControl struct {
	mu         sync.Mutex
	paused     bool
	pauseUntil time.Time
	stepOnce   bool              // when paused, allow exactly one iteration then re-block
	directives []agent.Directive // operator-injected guidance, drained by the loop
	wake       chan struct{}     // closed+replaced to broadcast a state change to Wait
	results    map[string]intervention.Result
	now        func() time.Time
}

func newRunControl() *runControl {
	return &runControl{wake: make(chan struct{}), results: map[string]intervention.Result{}, now: time.Now}
}

// broadcastLocked wakes every goroutine parked in Wait by closing the current
// wake channel and installing a fresh one. Caller holds mu.
func (rc *runControl) broadcastLocked() {
	close(rc.wake)
	rc.wake = make(chan struct{})
}

// Wait implements agent.Steerer: it blocks while the run is paused and returns
// when resumed, single-stepped, or ctx is done. A pending single-step consumes
// itself and returns nil while leaving the run paused, so the next iteration
// blocks again. Returns ctx.Err() if the context ends while parked — a paused
// run still honours halt/cancel/timeout.
func (rc *runControl) Wait(ctx context.Context) error {
	for {
		rc.mu.Lock()
		if !rc.paused {
			rc.mu.Unlock()
			return nil
		}
		if !rc.pauseUntil.IsZero() && !rc.now().Before(rc.pauseUntil) {
			rc.paused = false
			rc.stepOnce = false
			rc.pauseUntil = time.Time{}
			rc.broadcastLocked()
			rc.mu.Unlock()
			return nil
		}
		if rc.stepOnce {
			rc.stepOnce = false // advance exactly one iteration, stay paused after
			rc.mu.Unlock()
			return nil
		}
		wake := rc.wake
		var timer <-chan time.Time
		var t *time.Timer
		if !rc.pauseUntil.IsZero() {
			delay := time.Until(rc.pauseUntil)
			if delay <= 0 {
				delay = time.Nanosecond
			}
			t = time.NewTimer(delay)
			timer = t.C
		}
		rc.mu.Unlock()
		select {
		case <-ctx.Done():
			if t != nil {
				t.Stop()
			}
			return ctx.Err()
		case <-wake:
			if t != nil {
				t.Stop()
			}
			// state changed; re-evaluate
		case <-timer:
			if t != nil {
				t.Stop()
			}
			// lease expired; re-evaluate
		}
	}
}

// Drain implements agent.Steerer: returns and clears the queued directives in
// submission order. nil when none pending.
func (rc *runControl) Drain() []agent.Directive {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if len(rc.directives) == 0 {
		return nil
	}
	out := rc.directives
	rc.directives = nil
	return out
}

// pause parks the run at the next iteration boundary. Returns false (no-op) if
// already paused, so the caller can report idempotency.
func (rc *runControl) pause(until time.Time) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.paused && rc.pauseUntil.Equal(until) {
		return false
	}
	rc.paused = true
	rc.pauseUntil = until
	rc.broadcastLocked()
	return true
}

// resume clears the pause (and any pending step), letting the loop run freely.
// Returns false (no-op) if not paused.
func (rc *runControl) resume() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if !rc.paused {
		return false
	}
	rc.paused = false
	rc.stepOnce = false
	rc.pauseUntil = time.Time{}
	rc.broadcastLocked()
	return true
}

// step releases exactly one iteration then re-pauses. Pausing first if needed,
// so "step" works on a running agent too (pause-at-boundary, run one, re-pause).
func (rc *runControl) step() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.paused = true
	rc.stepOnce = true
	rc.pauseUntil = time.Time{}
	rc.broadcastLocked()
}

// inject queues a directive for the loop to fold into the next prompt. A paused
// run picks it up the moment it is resumed/stepped; a running run picks it up at
// the next iteration boundary.
func (rc *runControl) inject(directive string, note bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.directives = append(rc.directives, agent.Directive{Text: directive, Note: note})
}

// snapshot returns the current pause state and pending-directive count for the
// operator UI.
func (rc *runControl) snapshot() (paused bool, pending int, until time.Time) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.paused, len(rc.directives), rc.pauseUntil
}

func (rc *runControl) resultForKey(key string) (intervention.Result, bool) {
	if key == "" {
		return intervention.Result{}, false
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	res, ok := rc.results[key]
	return res, ok
}

func (rc *runControl) rememberResult(key string, res intervention.Result) {
	if key == "" {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.results[key] = res
}

// ----- kernel-facing operations -----

// controlFor returns the steering surface for a live run, or nil if there is no
// such active run (finished, never existed, wrong id).
func (k *Kernel) controlFor(corr string) *runControl {
	k.steersMu.Lock()
	defer k.steersMu.Unlock()
	return k.steers[corr]
}

// PauseRun parks a running agent at its next iteration boundary (M608). Returns
// true if a matching live run was found, false otherwise. Idempotent at the
// event level: a second pause on an already-paused run still returns true (the
// run is paused) but emits no duplicate event.
