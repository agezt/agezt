// SPDX-License-Identifier: MIT
package controlplane

import (
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the state's ports — the APPLICATION fills them from
// the kernel handle.
//
// The test pins that binding end to end: the store is the DAEMON's own, and
// the daemon-level binding answers for it.
func TestTheAdapterBindsTheStatePortsToTheDaemon(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(root, t.TempDir())

	if got := s.statePorts().Store(); got == nil {
		t.Fatal("the daemon's state ports hand over no store — the daemon holds one")
	} else if got != root.State() {
		t.Fatal("the daemon's state ports hand over a different store than the daemon's kernel holds")
	}
}
