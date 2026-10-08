// SPDX-License-Identifier: MIT
package controlplane

// nativeTerminalWrite is separate from unconditional cleanup ownership. Its
// accepted callbacks run only after the response writer returns, never on panic.
type nativeTerminalWrite struct{ nativeTerminalCleanup }

func (s *nativeTerminalWrite) After(callback func()) bool { return s.Defer(callback) }
func (s *nativeTerminalWrite) finishWrite()               { s.release() }
func (s *nativeTerminalWrite) discard() {
	s.mu.Lock()
	s.closed = true
	s.callbacks = nil
	s.mu.Unlock()
}
