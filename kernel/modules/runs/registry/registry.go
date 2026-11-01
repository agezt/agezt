// SPDX-License-Identifier: MIT

// Package registry owns the live-run registry: the correlations that are
// running right now, their cancel functions, the halt latch, the drain
// accounting that lets the daemon wait for them to settle, and the suspend
// latch that says the daemon is on its way out.
//
// The target architecture names this data explicitly: the runs module owns
// "run registry, resume tickets, steering/interventions". The resume tickets
// moved in W3.76a, the steering in W3.76b — this is the third of the three.
package registry

import (
	"context"
	"sync"
	"sync/atomic"
)

// Registry is the runs module's live-run registry.
//
// The LOCK DISCIPLINE is the store's, and it is explicit: a caller takes the
// lock, does its compound work and releases it. The daemon's compound
// cleanups must hold the registry's lock across the fan-out, tree, steer and
// spawn locks, because that lock order STARTS at the registry's — so the lock
// is part of the store's surface rather than a hidden field.
//
// Every verb below expects the lock to be held. The waitgroup and the
// suspend latch have their own synchronisation and are safe on their own.
type Registry struct {
	mu      sync.Mutex
	entries map[string]context.CancelFunc
	halted  bool
	wg      sync.WaitGroup
	// suspending latches true when the daemon begins tearing itself down for
	// a restart (M1002). A run cancelled while this is set is treated as
	// INTERRUPTED (its ticket is kept for resume) rather than
	// operator-cancelled (ticket deleted). Never cleared — the process is on
	// its way out.
	suspending atomic.Bool
}

// New returns an empty registry: no live runs, not halted, nothing draining.
func New() *Registry {
	return &Registry{entries: map[string]context.CancelFunc{}}
}

// Lock takes the registry's lock. Every state verb below expects it.
func (r *Registry) Lock() { r.mu.Lock() }

// Unlock releases the registry's lock.
func (r *Registry) Unlock() { r.mu.Unlock() }

// Halted reports whether the daemon is halted: new runs are refused.
func (r *Registry) Halted() bool { return r.halted }

// SetHalted latches (or clears) the halt flag.
func (r *Registry) SetHalted(halted bool) { r.halted = halted }

// Running reports whether the correlation is registered and therefore live.
func (r *Registry) Running(corr string) bool {
	_, ok := r.entries[corr]
	return ok
}

// Register records a correlation's cancel function. Registering a
// correlation that is already live would overwrite the first run's cancel,
// leaving it uncancellable — the caller checks Running first (M480).
func (r *Registry) Register(corr string, cancel context.CancelFunc) {
	r.entries[corr] = cancel
}

// Unregister forgets a correlation. The cancel function is NOT called: the
// caller owns when a run is cancelled.
func (r *Registry) Unregister(corr string) { delete(r.entries, corr) }

// Cancel forgets a correlation and returns its cancel function, if it was
// live. The caller invokes the returned function.
func (r *Registry) Cancel(corr string) (context.CancelFunc, bool) {
	cancel, ok := r.entries[corr]
	if ok {
		delete(r.entries, corr)
	}
	return cancel, ok
}

// Count is the number of live correlations.
func (r *Registry) Count() int { return len(r.entries) }

// IDs are the live correlations, in no particular order.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.entries))
	for corr := range r.entries {
		ids = append(ids, corr)
	}
	return ids
}

// Cancels are the live correlations' cancel functions.
func (r *Registry) Cancels() []context.CancelFunc {
	cancels := make([]context.CancelFunc, 0, len(r.entries))
	for _, cancel := range r.entries {
		cancels = append(cancels, cancel)
	}
	return cancels
}

// HaltAll latches the halt flag, takes every live correlation's cancel
// function and clears the registry. The caller invokes the returned
// functions: cancelling under the registry's lock would run a run's
// cancellation while the registry is held.
func (r *Registry) HaltAll() []context.CancelFunc {
	r.halted = true
	cancels := r.Cancels()
	// Drop every entry from the live registry. The map cannot be reassigned
	// (it is an interface-implemented field), but delete-everything has the
	// same effect: the next registration is the only writer of new entries,
	// and it does not depend on pre-existing entries being absent.
	for corr := range r.entries {
		delete(r.entries, corr)
	}
	return cancels
}

// Add counts a run towards the drain: the daemon waits for these to settle
// before it tears down the stores they write to (M883).
func (r *Registry) Add(n int) { r.wg.Add(n) }

// Done releases a run from the drain count. It is the LAST step a run takes
// (after every map / mutex / spawn-cancel cleanup), so the counter tracks the
// active set monotonically.
func (r *Registry) Done() { r.wg.Done() }

// Wait blocks until every counted run has settled.
func (r *Registry) Wait() { r.wg.Wait() }

// Suspending is the latch that says the daemon is tearing itself down for a
// restart. The daemon CASes it; the reader only asks.
func (r *Registry) Suspending() *atomic.Bool { return &r.suspending }
