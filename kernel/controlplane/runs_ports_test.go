// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the runs family's ports — the APPLICATION fills them
// from the kernel handle.
//
// The test pins that binding end to end: the daemon-level binding really
// STARTS the daemon's run and brings the answer back. A binding that answered
// for nobody would refuse every launch, and the surfaces would report the run
// capability as unavailable.
func TestTheAdapterBindsTheRunPortsToTheDaemon(t *testing.T) {
	provider := mock.New(mock.FinalText("the daemon's answer"))
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	answer, err := s.runPorts().Launch(context.Background(), root.NewCorrelation(), "do the thing", nil)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if answer != "the daemon's answer" {
		t.Fatalf("the daemon's run ports answered %q, want the daemon's own answer", answer)
	}
}
