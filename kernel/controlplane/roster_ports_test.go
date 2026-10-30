// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the roster's ports — the APPLICATION fills them from
// the kernel handle. What the host keeps is the binding: which kernel answers
// the roster's questions on this daemon.
//
// The test pins that binding: the agents the kernel holds are the agents the
// adapter's ports read, and they are the agents the daemon's roster list
// serves. A binding that answered from another roster would leave the
// operator looking at a list the daemon does not act on.
func TestTheAdapterBindsTheRosterPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	if _, err := k.AddProfile(roster.Profile{Slug: "lead", Enabled: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	s := NewServer(binding.ScopeFromKernel(k, dir))

	// The adapter's own ports read the kernel's agents.
	p, ok := s.rosterPorts().Get("lead")
	if !ok || p.Slug != "lead" {
		t.Fatalf("the adapter's roster ports Get = %+v %v, want the kernel's agent", p, ok)
	}
	list := s.rosterPorts().List()
	if len(list) != 1 || list[0].Slug != "lead" {
		t.Fatalf("the adapter's roster ports List = %+v, want the kernel's agent", list)
	}

	// And the daemon's roster list serves those same agents.
	out, err := s.rosterListService().List(context.Background(), approster.ListInput{})
	if err != nil {
		t.Fatalf("roster list: %v", err)
	}
	if out.Total != 1 || len(out.Profiles) != 1 || out.Profiles[0].Slug != "lead" {
		t.Fatalf("roster list = %+v, want the kernel's agent", out)
	}
}
