// SPDX-License-Identifier: MIT

package plugin

// White-box test for Close's robustness on a half-initialized Plugin
// (M183): Close must not nil-panic on a Plugin whose child process was
// never started (no cmd / no stdin), and must still mark it dead and
// drain any pending waiters.

import (
	"testing"
)

func TestClose_SafeOnUnstartedPlugin(t *testing.T) {
	p := &Plugin{
		pending:  make(map[string]chan *Response),
		progress: make(map[string]func(string)),
		cbSem:    make(chan struct{}, 1),
		// cmd and stdin deliberately nil — the process never started.
	}
	// A waiter must be unblocked (channel closed) by Close even on this
	// degenerate plugin.
	ch := make(chan *Response, 1)
	p.pending["q-1"] = ch

	if err := p.Close(); err != nil {
		t.Fatalf("Close on unstarted plugin: %v", err)
	}
	if !p.dead.Load() {
		t.Error("Close did not mark the plugin dead")
	}
	if _, ok := <-ch; ok {
		t.Error("pending channel not closed by Close")
	}
	// Idempotent: a second Close is a no-op, still no panic.
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
