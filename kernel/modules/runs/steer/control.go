// SPDX-License-Identifier: MIT

// Package steer is the runs module's steering face: the per-run steering
// surface a live run exposes, and the service that drives it.
//
// # WHY IT LIVES HERE
//
// The target architecture gives the runs module the "run registry, resume
// tickets, steering/interventions". W3.76a took the durable tickets; this
// slice takes the steering. The split follows the shape the family already
// had:
//
//   - the GRAMMAR (the wire vocabulary of an intervention) is in
//     kernel/modules/runs/intervention — it is pure data, and it is what a
//     host sends;
//   - the CONTROL (the per-run state machine: paused, stepped, the queued
//     directives, the remembered results) is here — it IS the run's steering
//     state;
//   - the SERVICE (pause, resume, step, steer, observe, intervene) is here
//     too — it is the runs module's business.
//
// The host keeps the doors: it owns the live runs, it publishes the events,
// and it cancels. The service asks for those through ports, so the module
// never names the daemon.
package steer

import (
	"context"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/modules/runs/intervention"
)

// Control is the per-run steering surface. It implements agent.Steerer
// (Wait + Drain) for the loop and exposes Pause/Resume/Step/Inject for the
// operator. The zero value is not usable — construct with NewControl.
type Control struct {
	mu         sync.Mutex
	paused     bool
	pauseUntil time.Time
	stepOnce   bool              // when paused, allow exactly one iteration then re-block
	directives []agent.Directive // operator-injected guidance, drained by the loop
	wake       chan struct{}     // closed+replaced to broadcast a state change to Wait
	results    map[string]intervention.Result
	now        func() time.Time
}

// NewControl returns a usable steering surface.
func NewControl() *Control {
	return &Control{wake: make(chan struct{}), results: map[string]intervention.Result{}, now: time.Now}
}

// broadcastLocked wakes every goroutine parked in Wait by closing the current
// wake channel and installing a fresh one. Caller holds mu.
func (rc *Control) broadcastLocked() {
	close(rc.wake)
	rc.wake = make(chan struct{})
}

// Wait implements agent.Steerer: it blocks while the run is paused and returns
// when resumed, single-stepped, or ctx is done. A pending single-step consumes
// itself and returns nil while leaving the run paused, so the next iteration
// blocks again. Returns ctx.Err() if the context ends while parked — a paused
// run still honours halt/cancel/timeout.
func (rc *Control) Wait(ctx context.Context) error {
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
func (rc *Control) Drain() []agent.Directive {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if len(rc.directives) == 0 {
		return nil
	}
	out := rc.directives
	rc.directives = nil
	return out
}

// Pause parks the run at the next iteration boundary. Returns false (no-op) if
// already paused, so the caller can report idempotency.
func (rc *Control) Pause(until time.Time) bool {
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

// Resume clears the pause (and any pending step), letting the loop run freely.
// Returns false (no-op) if not paused.
func (rc *Control) Resume() bool {
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

// Step releases exactly one iteration then re-pauses. Pausing first if needed,
// so "step" works on a running agent too (pause-at-boundary, run one, re-pause).
func (rc *Control) Step() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.paused = true
	rc.stepOnce = true
	rc.pauseUntil = time.Time{}
	rc.broadcastLocked()
}

// Inject queues a directive for the loop to fold into the next prompt. A paused
// run picks it up the moment it is resumed/stepped; a running run picks it up at
// the next iteration boundary.
func (rc *Control) Inject(directive string, note bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.directives = append(rc.directives, agent.Directive{Text: directive, Note: note})
}

// Snapshot returns the current pause state and pending-directive count for the
// operator UI.
func (rc *Control) Snapshot() (paused bool, pending int, until time.Time) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.paused, len(rc.directives), rc.pauseUntil
}

// ResultForKey returns the remembered result for an idempotency key.
func (rc *Control) ResultForKey(key string) (intervention.Result, bool) {
	if key == "" {
		return intervention.Result{}, false
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	res, ok := rc.results[key]
	return res, ok
}

// RememberResult stores a result under its idempotency key.
func (rc *Control) RememberResult(key string, res intervention.Result) {
	if key == "" {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.results[key] = res
}
