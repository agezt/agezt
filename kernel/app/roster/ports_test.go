// SPDX-License-Identifier: MIT

package roster

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The roster's profile ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: every question the roster asks about an
// agent comes back with the kernel's own answer, not with a stand-in. A port
// that answered "no agents" for a kernel that holds one would leave the roster
// working on a roster that is not there.
func TestTheProfilePortsReachTheKernelProfiles(t *testing.T) {
	dir := t.TempDir()
	store, err := core.Open(filepath.Join(dir, "roster"))
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	standingStore, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("standing.Open: %v", err)
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(),
		Roster:   store,
		Standing: standingStore,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := ProfilePortsFromKernel(k)

	// A read of an agent that is not there says so.
	if _, ok := ports.Get("nobody"); ok {
		t.Error("Get(nobody) = found, want not found")
	}

	// Adding an agent: the port's answer is the kernel's saved profile.
	saved, err := ports.Add(agentapi.Profile{Slug: "lead", Soul: "s"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if saved.Slug != "lead" {
		t.Fatalf("Add slug = %q, want lead", saved.Slug)
	}

	// Reading it back: the port sees the agent the kernel holds.
	got, ok := ports.Get("lead")
	if !ok {
		t.Fatal("Get(lead) = not found, want the agent the port added")
	}
	if got.Slug != "lead" {
		t.Errorf("Get slug = %q, want lead", got.Slug)
	}

	// Listing them: the port sees the whole roster.
	list := ports.List()
	if len(list) != 1 || list[0].Slug != "lead" {
		t.Fatalf("List = %+v, want the one agent the port added", list)
	}

	// Editing one: the mutation lands and comes back.
	edited, found, err := ports.Update("lead", func(p *agentapi.Profile) { p.Soul = "s2" })
	if err != nil || !found {
		t.Fatalf("Update = %v %v %v, want the agent edited", edited, found, err)
	}
	if edited.Soul != "s2" {
		t.Errorf("Update soul = %q, want s2", edited.Soul)
	}
	if reread, _ := ports.Get("lead"); reread.Soul != "s2" {
		t.Errorf("Get soul = %q, want the edit to have landed", reread.Soul)
	}

	// Pausing one: the port's answer is the paused profile.
	paused, err := ports.SetEnabled("lead", false)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if paused.Enabled {
		t.Error("SetEnabled(false) left the agent enabled")
	}
	resumed, err := ports.SetEnabled("lead", true)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if !resumed.Enabled {
		t.Error("SetEnabled(true) left the agent paused")
	}

	// Retiring one: the reason the host gave reaches the profile.
	retired, err := ports.SetRetired("lead", true, "done")
	if err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	if !retired.Retired || retired.RetiredReason != "done" {
		t.Errorf("SetRetired = %+v, want retired with the reason", retired)
	}
	revived, err := ports.SetRetired("lead", false)
	if err != nil {
		t.Fatalf("SetRetired(revive): %v", err)
	}
	if revived.Retired {
		t.Error("SetRetired(false) left the agent retired")
	}

	// What depends on the agent: the standing orders that fire as it.
	if _, err := k.AddStanding(standing.Order{
		Name:     "watch",
		Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "x.y"}},
		Agent:    "lead",
		Plan:     "do",
	}); err != nil {
		t.Fatalf("AddStanding: %v", err)
	}
	impact := ports.Impact("lead")
	if len(impact) != 1 || !strings.HasPrefix(impact[0], "watch (") {
		t.Fatalf("Impact = %v, want the one standing order that fires as the agent", impact)
	}
	if other := ports.Impact("stranger"); len(other) != 0 {
		t.Errorf("Impact(stranger) = %v, want none", other)
	}

	// Removing one: the port says the agent was there.
	removed, err := ports.Remove("lead")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Error("Remove = false, want the agent that was there to be removed")
	}
	if _, ok := ports.Get("lead"); ok {
		t.Error("Get(lead) = found after Remove, want gone")
	}
}
