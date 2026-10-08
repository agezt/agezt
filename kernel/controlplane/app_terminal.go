// SPDX-License-Identifier: MIT
package controlplane

import "sync"

// nativeTerminalCleanup releases resources after result/error socket delivery,
// including failed writes and panics. Callbacks run outside the ownership lock.
type nativeTerminalCleanup struct {
	mu        sync.Mutex
	closed    bool
	callbacks []func()
}

func (s *nativeTerminalCleanup) Defer(cleanup func()) bool {
	if cleanup == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.callbacks = append(s.callbacks, cleanup)
	return true
}

func (s *nativeTerminalCleanup) release() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	callbacks := s.callbacks
	s.callbacks = nil
	s.mu.Unlock()
	// Each defer still runs if a later cleanup panics; ordering matches stack
	// unwinding and repeated release cannot execute a callback twice.
	for _, cleanup := range callbacks {
		defer cleanup()
	}
}
