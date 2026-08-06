// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The provenance walk itself (Why / Causes / ParentOf) now lives in
// kernel/journal (Phase 3.1 C) and is exercised by
// kernel/journal/provenance_test.go. What remains here is the thin
// delegation contract: the Kernel methods must reach the SAME journal the
// bus appends to, so `agt why` sees bus-published events.

// openCausesKernel spins a real kernel over a temp journal. Shared by the
// delegation test below and other white-box tests (reaper, foldRunTools)
// that need a kernel without the rest of the agent loop.
func openCausesKernel(t *testing.T) *Kernel {
	t.Helper()
	k, err := Open(Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestCauses_DelegatesToJournal publishes a cross-correlation causation
// chain through the kernel's Bus and asserts the Kernel-level Why/Causes/
// ParentOf delegators answer from that journal.
func TestCauses_DelegatesToJournal(t *testing.T) {
	k := openCausesKernel(t)

	tick, err := k.Bus().Publish(event.Spec{
		Subject: "test.causation", Kind: event.KindPulseTick, Actor: "test",
		CorrelationID: "pulse-tick-1",
	})
	if err != nil {
		t.Fatalf("publish tick: %v", err)
	}
	// Different correlation, caused by the tick — reachable only via causation.
	initiative, err := k.Bus().Publish(event.Spec{
		Subject: "test.causation", Kind: event.KindInitiativeTaken, Actor: "test",
		CorrelationID: "pulse-delta-1", CausationID: tick.ID,
	})
	if err != nil {
		t.Fatalf("publish initiative: %v", err)
	}

	whyEvents, err := k.Why(initiative.ID)
	if err != nil {
		t.Fatalf("Why: %v", err)
	}
	if len(whyEvents) == 0 {
		t.Fatal("Why returned no events for a bus-published id")
	}

	chain, err := k.Causes(initiative.ID)
	if err != nil {
		t.Fatalf("Causes: %v", err)
	}
	if len(chain) != 2 || chain[0].ID != tick.ID || chain[1].ID != initiative.ID {
		t.Fatalf("Causes delegation broken: got %d events, want tick → initiative", len(chain))
	}

	if got := k.ParentOf("no-such-child"); got != "" {
		t.Errorf("ParentOf(no-such-child) = %q want empty", got)
	}
}
