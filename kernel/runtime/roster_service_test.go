// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's face over the roster service (W3.76n).
//
// The roster's WRITES are the agents module's: the module keeps the lifecycle,
// the events and the guardian rule, and the daemon keeps the names and
// forwards. These tests pin the two halves of that split — the daemon still
// answers every call, and the daemon no longer decides what a call means.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openWithRoster opens a kernel over a real roster store, the way the
// composition root does (the roster's file is the host's business).
func openWithRoster(t *testing.T, wire bool) (*runtime.Kernel, agentapi.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := roster.Open(filepath.Join(dir, "roster"))
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	cfg := runtime.Config{BaseDir: dir, Provider: mock.New(), Roster: store}
	if wire {
		cfg.RosterService = roster.NewService
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })
	return k, store
}

// TestTheDaemonForwardsTheRosterWrites: the daemon's names still answer, and
// the answer comes from the module — the agent lands in the store and the
// birth is announced on the kernel's bus.
func TestTheDaemonForwardsTheRosterWrites(t *testing.T) {
	k, store := openWithRoster(t, true)

	saved, err := k.AddProfile(agentapi.Profile{Slug: "worker"})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if saved.Slug != "worker" {
		t.Fatalf("AddProfile answered %+v, want the saved profile", saved)
	}
	if got, ok := store.Get("worker"); !ok || got.ID != saved.ID {
		t.Fatalf("the store holds %v (found=%v), want the profile the daemon answered", got, ok)
	}

	// The announcement travelled: the module published it, the kernel's bus
	// carried it and the kernel's journal holds it.
	seen := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindRosterCreated {
			seen++
		}
		return nil
	})
	if seen != 1 {
		t.Errorf("the journal holds %d roster.created events, want 1 — the module publishes what happened to the agent", seen)
	}
}

// TestTheDaemonRefusesTheWritesWithoutAService: a host that handed over no
// builder has no roster lifecycle. The refusal is the contract's row, and the
// daemon's reads still answer "no agents" rather than panicking.
func TestTheDaemonRefusesTheWritesWithoutAService(t *testing.T) {
	k, _ := openWithRoster(t, false)

	if _, err := k.AddProfile(agentapi.Profile{Slug: "worker"}); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("AddProfile error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, err := k.SetProfileEnabled("worker", false); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("SetProfileEnabled error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, err := k.SetProfileRetired("worker", true); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("SetProfileRetired error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, _, err := k.UpdateProfile("worker", func(p *agentapi.Profile) {}); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("UpdateProfile error = %v, want %v", err, agentapi.ErrNoRoster)
	}
	if _, err := k.RemoveProfile("worker"); !errors.Is(err, agentapi.ErrNoRoster) {
		t.Errorf("RemoveProfile error = %v, want %v", err, agentapi.ErrNoRoster)
	}

	// The READS are the host's nil-safe view over the contract's port: they
	// need no service and they answer.
	if _, ok := k.RosterGet("worker"); ok {
		t.Error("RosterGet found a worker in a roster that has none")
	}
	if got := k.RosterList(); len(got) != 0 {
		t.Errorf("RosterList = %v, want no agents", got)
	}
}

// TestTheDaemonStillReadsTheRosterItself: the reads are the host's face over
// the contract's port, and they answer with the store's rows.
func TestTheDaemonStillReadsTheRosterItself(t *testing.T) {
	k, store := openWithRoster(t, true)

	if _, err := k.AddProfile(agentapi.Profile{Slug: "worker"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, ok := k.RosterGet("worker")
	if !ok {
		t.Fatal("RosterGet did not find the agent the daemon just added")
	}
	if got.Slug != "worker" {
		t.Errorf("RosterGet answered %+v, want the worker", got)
	}
	if list := k.RosterList(); len(list) != 1 {
		t.Errorf("RosterList = %d rows, want 1", len(list))
	}
	if port := k.Roster(); port == nil {
		t.Error("the roster port is nil — the host holds the contract's port")
	}
	_ = store
}

// TestTheDaemonStillAsksWhatDependsOnAnAgent: AgentImpact stayed with the
// host, because the standing orders are another domain's rows and a module
// may not name another module's data.
func TestTheDaemonStillAsksWhatDependsOnAnAgent(t *testing.T) {
	k, _ := openWithRoster(t, true)

	if got := k.AgentImpact("worker"); got != nil {
		t.Errorf("AgentImpact = %v, want nothing — no standing order names that agent", got)
	}
	if got := k.AgentImpact("  "); got != nil {
		t.Errorf("AgentImpact(blank) = %v, want nothing", got)
	}
}

// TestTheRunEngineWritesTheRosterThroughTheModule: the composition root
// wires the agents module's roster service into the run engine, so a run
// that completes retires its agent THROUGH the module (W3.76o) — the run
// engine does not ask the host to ask the module.
func TestTheRunEngineWritesTheRosterThroughTheModule(t *testing.T) {
	k, store := openWithRoster(t, true)

	if _, err := k.AddProfile(agentapi.Profile{
		Slug:      "worker",
		Lifecycle: agentapi.AgentLifecycle{RetireOnComplete: true},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	ctx := runctx.WithAgentIdent(context.Background(), "worker", 0)
	k.CompleteAgentLifecycle(ctx, "corr-1")

	got, ok := store.Get("worker")
	if !ok {
		t.Fatal("the agent is gone from the roster")
	}
	if !got.Retired {
		t.Error("the agent was not retired — the run engine did not reach the roster")
	}
}
