// SPDX-License-Identifier: MIT

// Pulse engine observation: PendingAsks + ResolveAsk + tickOnce + AddObserver + RemoveObserver + safePoll + FlushDigest + safeFlushDigest.
// Code extracted from engine.go during the Day-54 god-file split. Public API unchanged.
package pulse


import (
	"context"
	"fmt"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)


// PendingAsks returns the queued asks awaiting an operator verdict (M1001), newest
// first, as plain maps the control plane can return without importing this package.
func (e *Engine) PendingAsks() []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]map[string]any, 0, len(e.asks))
	for _, a := range e.asks {
		out = append(out, map[string]any{
			"issue_key":  a.IssueKey,
			"source":     a.Source,
			"kind":       a.Kind,
			"summary":    a.Summary,
			"reason":     a.Reason,
			"score":      a.Score,
			"ts_unix_ms": a.TS,
		})
	}
	// Newest first; a stable order keeps the UI from reshuffling on each poll.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j]["ts_unix_ms"].(int64) > out[i]["ts_unix_ms"].(int64) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ResolveAsk settles a pending ask (M1001). approve=true re-emits the original signal
// onto pulse.initiative.act — the exact path act-mode takes, so the responder (when
// enabled) fires a governed run; approve=false just drops it. Returns whether the key
// was found and, on approval, whether the act event was emitted.
func (e *Engine) ResolveAsk(issueKey string, approve bool) (found, acted bool) {
	e.mu.Lock()
	a := e.asks[issueKey]
	if a != nil {
		delete(e.asks, issueKey)
	}
	e.mu.Unlock()
	if a == nil {
		return false, false
	}
	if approve {
		// Re-emit verbatim onto the act subject the responder binds to. The engine still
		// takes no action itself — it only promotes ask→act; the governed standing runner
		// does the rest (and does nothing if the operator left the responder disabled).
		e.publish(event.KindInitiativeAct, "pulse.initiative.act", "pulse-"+ulid.New(), "", a.payload)
		return true, true
	}
	return true, false
}

// tickOnce executes a single heartbeat: publish the tick, poll observers, and
// run each delta through salience → initiative → briefing. Exposed for
// deterministic tests (drive beats without a real ticker).
func (e *Engine) tickOnce(ctx context.Context) {
	e.mu.Lock()
	e.ticks++
	n := e.ticks
	e.lastTickMS = e.now().UnixMilli()
	// Snapshot the observers under the lock — AddObserver (M767) can append from the
	// control-plane goroutine while this beat iterates them.
	obs := make([]Observer, len(e.observers))
	copy(obs, e.observers)
	e.mu.Unlock()

	tickEv, _ := e.publish(event.KindPulseTick, "pulse.tick", "pulse-"+ulid.New(), "", map[string]any{
		"beat":      n,
		"observers": len(obs),
	})
	tickID := ""
	if tickEv != nil {
		tickID = tickEv.ID
	}

	for _, o := range obs {
		e.safePoll(ctx, o, tickID)
	}

	if n%int64(e.digestEvery) == 0 {
		e.safeFlushDigest()
	}
}

// AddObserver registers a new observer at runtime (M767) — e.g. an operator adding a
// disk watch from the console. Appended under the lock; the next beat picks it up
// (tickOnce snapshots the slice under the same lock). Returns the observer's name.
func (e *Engine) AddObserver(o Observer) string {
	e.mu.Lock()
	e.observers = append(e.observers, o)
	e.removable[o] = true
	e.mu.Unlock()
	return o.Name()
}

// RemoveObserver removes runtime-added observers whose Name() matches (M769) — the
// inverse of AddObserver, so a console-added disk or command watch can be stopped
// without restarting the daemon. Only observers registered via AddObserver are
// removable; startup observers (self:health and any AGEZT_PULSE_* probes) are never
// removed, even on a name collision. Returns how many were dropped (0 if none matched
// or the name belongs only to a permanent observer). Takes effect on the next beat
// (tickOnce snapshots the slice under this same lock).
func (e *Engine) RemoveObserver(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	kept := e.observers[:0:0]
	removed := 0
	for _, o := range e.observers {
		if o.Name() == name && e.removable[o] {
			delete(e.removable, o)
			removed++
			continue
		}
		kept = append(kept, o)
	}
	e.observers = kept
	return removed
}

// safePoll polls one observer and runs its deltas through the pipeline, recovering
// from any panic so a buggy observer, a panicking provider in the salience refine, or
// a panicking briefing sink can never crash the whole daemon (M423). The pulse loop
// runs on a single resident goroutine with no recovering frame, so without this an
// observer/provider/sink panic terminates the process — every channel, the control
// plane, and all in-flight runs with it. Mirrors kernel/standing's safeFire and
// kernel/cadence's fireOne. The panic is journaled so it stays diagnosable.
func (e *Engine) safePoll(ctx context.Context, obs Observer, tickID string) {
	defer func() {
		if r := recover(); r != nil {
			e.publish(event.KindObserverDelta, "pulse.observer."+obs.Name(), "pulse-"+ulid.New(), tickID, map[string]any{
				"observer": obs.Name(),
				"error":    fmt.Sprintf("panic (contained): %v", r),
			})
		}
	}()
	deltas, err := obs.Poll(ctx)
	if err != nil {
		e.publish(event.KindObserverDelta, "pulse.observer."+obs.Name(), "pulse-"+ulid.New(), tickID, map[string]any{
			"observer": obs.Name(),
			"error":    err.Error(),
		})
		return
	}
	for _, d := range deltas {
		e.process(ctx, d, tickID)
	}
}

// FlushDigest delivers any accumulated digest items immediately (M761) instead of
// waiting for the periodic flush (every digestEvery beats) — "surface what you've been
// holding". Returns the number of items flushed (0 if the digest was empty). Safe to
// call from any goroutine: the digest is swapped under the lock, so a concurrent
// periodic flush can't double-deliver.
func (e *Engine) FlushDigest() int { return e.safeFlushDigest() }
