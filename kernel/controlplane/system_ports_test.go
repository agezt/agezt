// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/app/system"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the daemon overview's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon-level binding really
// READS the daemon, and the overview built on it reports the daemon it was
// bound to.
func TestTheAdapterBindsTheDaemonOverviewPortsToTheDaemon(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	ports := s.systemPorts()
	if !ports.Ready() {
		t.Fatal("the adapter's binding holds no daemon — the overview would report nothing")
	}
	if ports.Started().IsZero() {
		t.Fatal("the adapter's binding does not reach the daemon's start time")
	}
	if ports.ToolCount() != len(root.Tools()) {
		t.Fatalf("the adapter's binding reports %d tools, want %d", ports.ToolCount(), len(root.Tools()))
	}
	if ports.Gate() == nil {
		t.Fatal("the adapter's binding holds no gate — the daemon's halt and resume have nowhere to go")
	}

	status, err := s.systemService().Status(context.Background(), system.StatusInput{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Daemon == "" {
		t.Fatal("the overview reported no daemon")
	}
	if status.Tools != len(root.Tools()) {
		t.Fatalf("the overview reports %d tools, want %d", status.Tools, len(root.Tools()))
	}
	if status.JournalHead < 0 {
		t.Fatalf("the overview reports a journal head of %d", status.JournalHead)
	}
}
