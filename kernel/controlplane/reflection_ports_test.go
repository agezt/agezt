// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the reflection family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the engine is the DAEMON's own, and
// the daemon-level binding hands exactly that engine over.
//
// The reflection engine is not a daemon's file — the composition root always
// builds one — so there is no "daemon without an engine" to test against: the
// daemon that WAS given a kernel always holds an engine.
func TestTheAdapterBindsTheReflectionPortsToTheDaemon(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	if got := s.reflectionPorts().Engine(); got == nil {
		t.Fatal("the daemon's reflection ports hand over no engine — the engine never reached the family")
	}
	if s.reflectionPorts().Engine() != root.Reflect() {
		t.Fatal("the daemon's reflection ports hand over a different engine than the daemon's kernel holds")
	}
}
