// SPDX-License-Identifier: MIT

package runtime

// Live run steering (M608) — let an operator fly a running agent from the
// cockpit: pause it at the next safe boundary, single-step one iteration,
// inject a directive that folds into the next prompt, or resume. The agent loop
// (kernel/agent) consults a per-run *runControl (registered in k.steers) at the
// top of every iteration via the agent.Steerer interface. The control methods
// here are driven by the control plane from another goroutine while the loop
// runs, so every field is mutex-guarded and pause-blocking uses a broadcast
// channel that honours the run's context (a paused run is still killable).

import (
	"context"
	"maps"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// runControl is the per-run steering surface. It implements agent.Steerer
// (Wait + Drain) for the loop and exposes Pause/Resume/Step/Inject for the
// operator. The zero value is not usable — construct with newRunControl.
type runControl struct {
	mu         sync.Mutex
	paused     bool
	stepOnce   bool              // when paused, allow exactly one iteration then re-block
	directives []agent.Directive // operator-injected guidance, drained by the loop
	wake       chan struct{}     // closed+replaced to broadcast a state change to Wait
}

func newRunControl() *runControl {
	return &runControl{wake: make(chan struct{})}
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
		if rc.stepOnce {
			rc.stepOnce = false // advance exactly one iteration, stay paused after
			rc.mu.Unlock()
			return nil
		}
		wake := rc.wake
		rc.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
			// state changed; re-evaluate
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
func (rc *runControl) pause() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.paused {
		return false
	}
	rc.paused = true
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
func (rc *runControl) snapshot() (paused bool, pending int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.paused, len(rc.directives)
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
func (k *Kernel) PauseRun(corr string) bool {
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	if rc.pause() {
		k.publishSteer(corr, event.KindRunPaused, nil)
	}
	return true
}

// ResumeRun lets a paused agent run freely again (M608). Returns true if the
// run exists; emits run.resumed only on an actual state change.
func (k *Kernel) ResumeRun(corr string) bool {
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	if rc.resume() {
		k.publishSteer(corr, event.KindRunResumed, nil)
	}
	return true
}

// StepRun advances a run by exactly one iteration then re-pauses it (M608),
// pausing first if it was running. Returns true if the run exists.
func (k *Kernel) StepRun(corr string) bool {
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	rc.step()
	k.publishSteer(corr, event.KindRunStepped, nil)
	return true
}

// SteerRun injects an operator directive into a running agent (M608); the loop
// folds it into the conversation as a fresh user turn at the next iteration
// boundary (and emits run.steered when it takes effect). Returns true if the
// run exists. An empty directive is rejected (false) so the UI can validate.
// note=true marks a soft "BTW" (read it, finish the current step, stay on task)
// vs a forceful steer that re-prioritises (M962).
func (k *Kernel) SteerRun(corr, directive string, note bool) bool {
	if directive == "" {
		return false
	}
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	rc.inject(directive, note)
	return true
}

// RunControlState reports a run's live steering state for the operator UI:
// paused flag + count of directives queued but not yet folded. ok=false when
// there is no such active run.
func (k *Kernel) RunControlState(corr string) (paused bool, pending int, ok bool) {
	rc := k.controlFor(corr)
	if rc == nil {
		return false, 0, false
	}
	paused, pending = rc.snapshot()
	return paused, pending, true
}

// publishSteer emits a steering control event correlated to the run, so the
// action shows up on the run's timeline and the live firehose. Best-effort —
// a bus error never blocks the control operation.
func (k *Kernel) publishSteer(corr string, kind event.Kind, extra map[string]any) {
	payload := map[string]any{"correlation_id": corr}
	maps.Copy(payload, extra)
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "kernel.steer",
		Kind:          kind,
		Actor:         "operator",
		CorrelationID: corr,
		Payload:       payload,
	})
}
