// SPDX-License-Identifier: MIT

package runner

// The run engine's live-run state door (W3.76ae).
//
// The run's setup and cleanup used to be the host's own verbs, and the run
// engine asked the host for them through its port — a round trip inside ONE
// module. The live-run state (the steering registry and the per-run
// registration) is the module's own, so the engine asks the module.
//
// These tests pin that: the engine asks the module's state, and a host that
// wired no state leaves the engine quiet instead of broken.

import (
	"os"
	"regexp"
	"testing"

	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/state"
)

// theRunStateEntriesAreNotTheHosts are the run-state operations the run
// engine's host port must NOT carry: the run's live state is the module's
// own, and a port entry that is a round trip inside one module is a promise
// the host never had to keep.
var theRunStateEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\t(SetupRunState|CleanupRunState|DeregisterRunSteer)\(`)

// TestTheRunEnginePortCarriesNoRunStateEntry pins the PORT: it promises the
// host's capabilities, not the module's own live-run state.
func TestTheRunEnginePortCarriesNoRunStateEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if theRunStateEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a run-state entry — the run's live state is the module's own (W3.76ae)")
	}
}

// testState returns a live-run state over a fresh registry — the smallest
// state a test can run a registration against.
func testState() *state.State {
	return state.New(state.Dependencies{Registry: registry.New()})
}

// TestTheRunEngineAsksTheModulesLiveRunState pins the wiring: the engine
// holds the module's state, not a host door.
func TestTheRunEngineAsksTheModulesLiveRunState(t *testing.T) {
	st := testState()
	r := New(&stubKernel{}, nil, st, testPrompt(), testPublisher(), testTickets())
	if r.runState != st {
		t.Fatal("the run engine does not hold the module's live-run state")
	}
}

// TestTheRunEngineRefusesWithoutALiveRunState pins the refusal: a host that
// wired no state leaves the engine quiet — it says the state is missing
// instead of registering nothing.
func TestTheRunEngineRefusesWithoutALiveRunState(t *testing.T) {
	r := New(&stubKernel{}, nil, nil, testPrompt(), testPublisher(), testTickets())
	if _, err := r.RunWith(t.Context(), "corr-1", "do the thing"); err == nil {
		t.Fatal("the engine ran without a live-run state")
	}
}
