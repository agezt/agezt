// SPDX-License-Identifier: MIT

package runner

// The run engine's TICKET LIFECYCLE door (W3.76ah).
//
// Who owns the durable resume ticket, when it is refreshed and when it is
// deleted is the RUNS MODULE's own (kernel/modules/runs/resume) — the tickets,
// the store and the rules that judge them live there. The daemon held three
// verbs over the operations (ClaimResumeTicket, FinalizeResumeTicket,
// ResumeCheckpointFn) and the run engine — which lives INSIDE the module —
// asked the HOST for them through its port: a round trip inside ONE module,
// three times.
//
// The engine holds the module's ticket lifecycle now and asks the module.
//
// These tests pin that: the port carries no ticket-lifecycle entry, the engine
// holds the module's service, and a host that wired none leaves the engine
// quiet instead of running a run whose ticket it does not own.

import (
	"os"
	"regexp"
	"testing"

	"github.com/agezt/agezt/kernel/modules/runs/resume"
)

// theTicketEntriesAreNotTheHosts are the ticket-lifecycle operations the run
// engine's host port must NOT carry: the ticket is the run's own durable
// state, and a port entry that is a round trip inside one module is a promise
// the host never had to keep.
var theTicketEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\t(ClaimResumeTicket|FinalizeResumeTicket|ResumeCheckpointFn)\(`)

// TestTheRunEnginePortCarriesNoTicketEntry pins the PORT: it promises the
// host's capabilities, not the module's own ticket lifecycle.
func TestTheRunEnginePortCarriesNoTicketEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if theTicketEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a ticket entry — the ticket's lifecycle is the module's (W3.76ah)")
	}
}

// TestTheRunEngineAsksTheModulesTicketLifecycle pins the wiring: the engine
// holds the module's service, and it CALLS the module — a field nobody asks is
// a field that promises nothing.
func TestTheRunEngineAsksTheModulesTicketLifecycle(t *testing.T) {
	svc := testTickets()
	r := New(&stubKernel{}, nil, testState(), testPrompt(), testPublisher(), svc)
	if r.tickets != svc {
		t.Fatal("the run engine does not hold the module's ticket lifecycle")
	}
	for _, call := range []string{
		"r.tickets.Claim(",
		"r.tickets.CheckpointFn(",
		"r.tickets.Finalize(",
	} {
		if !runnerPackageContains(t, call) {
			t.Errorf("the run engine must ask the module's ticket lifecycle (%s) — the ticket is the run's own state (W3.76ah)", call)
		}
	}
}

// TestTheRunEngineRefusesWithoutATicketLifecycle pins the refusal: a host
// that wired no ticket lifecycle leaves the engine quiet — it says the
// lifecycle is missing instead of running a run whose ticket it cannot own.
func TestTheRunEngineRefusesWithoutATicketLifecycle(t *testing.T) {
	r := New(&stubKernel{}, nil, testState(), testPrompt(), testPublisher(), nil)
	if _, err := r.RunWith(t.Context(), "corr-1", "do the thing"); err == nil {
		t.Fatal("the engine ran without a ticket lifecycle")
	}
}

// testTickets returns the module's ticket lifecycle over no store at all —
// the smallest lifecycle a test can run a run through. Resume off means no
// ticket is written, which is what a test that does not care about the
// ticket wants.
func testTickets() *resume.Service {
	return resume.New(resume.Dependencies{})
}
