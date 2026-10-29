// SPDX-License-Identifier: MIT
package controlplane

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the daemon lifecycle family's ports — the APPLICATION
// fills them from the kernel handle.
//
// The test pins that binding end to end: the daemon-level binding really
// DRAINS the daemon, and the daemon's own halted flag says so afterwards.
func TestTheAdapterBindsTheLifecyclePortsToTheDaemon(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())

	timedOut, active, err := s.lifecyclePorts().Drain(time.Second)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if timedOut || active != 0 {
		t.Fatalf("the daemon's drain answered %v/%d, want a clean drain", timedOut, active)
	}
	if !root.IsHalted() {
		t.Fatal("the daemon's drain did not halt the daemon")
	}
}
