// SPDX-License-Identifier: MIT

package main

import "sync"

// lateHolder publishes ONE late artifact to the control plane.
//
// The pulse engine, the live channels and the standing-order runner only exist
// AFTER the adapter is up, and the adapter has no phase in which the root binds
// them (the target architecture's P8: "wiring is declarative"). The root
// declares them here and hands the adapter a way to ASK; the answer is nil
// until the artifact is ready.
//
// The holder is synchronized: the root publishes on its own goroutine while
// the adapter's request goroutines ask.
type lateHolder[T any] struct {
	mu sync.RWMutex
	v  T
}

// get answers the artifact, or the zero value when it is not there yet.
func (h *lateHolder[T]) get() T {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.v
}

// set publishes the artifact.
func (h *lateHolder[T]) set(v T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.v = v
}
