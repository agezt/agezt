// SPDX-License-Identifier: MIT

package journal

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// appendProv appends an event with an explicit correlation + causation and
// returns the journal-assigned event (so its ID can be used as the next
// causation link). Moved with the provenance queries from kernel/runtime's
// causation tests (Phase 3.1 C).
func appendProv(t *testing.T, j *Journal, kind event.Kind, corr, causation string) *event.Event {
	t.Helper()
	ev, err := j.Append(event.Spec{
		Subject:       "test.causation",
		Kind:          kind,
		Actor:         "test",
		CorrelationID: corr,
		CausationID:   causation,
		Payload:       map[string]any{"k": string(kind)},
	})
	if err != nil {
		t.Fatalf("append %s: %v", kind, err)
	}
	return ev
}

// TestCauses_WalksCausationAcrossCorrelation is the gap-closing test for
// SPEC-01 §7.1. Pulse publishes a tick under one correlation, then an
// initiative under a DIFFERENT correlation that links back to the tick ONLY via
// causation_id (exactly how engine.go threads tickID). Why groups by
// correlation, so it cannot see the tick from the initiative; Causes follows
// causation_id and reaches it. This asserts both halves: the gap (Why misses
// the tick) and the fix (Causes recovers the provenance, root-first).
func TestCauses_WalksCausationAcrossCorrelation(t *testing.T) {
	j := newTestJournal(t, 0)

	tick := appendProv(t, j, event.KindPulseTick, "pulse-tick-1", "")
	// Different correlation, caused by the tick — mirrors engine.go's
	// publish(KindInitiativeTaken, corr, tickID, …).
	initiative := appendProv(t, j, event.KindInitiativeTaken, "pulse-delta-1", tick.ID)

	// The gap: Why on the initiative groups by its own correlation and never
	// reaches the originating tick (different correlation).
	whyEvents, err := j.Why(initiative.ID)
	if err != nil {
		t.Fatalf("Why: %v", err)
	}
	for _, e := range whyEvents {
		if e.ID == tick.ID {
			t.Fatalf("Why unexpectedly reached the tick — the correlation gap this test relies on is gone")
		}
	}

	// The fix: Causes follows causation_id across the correlation boundary.
	chain, err := j.Causes(initiative.ID)
	if err != nil {
		t.Fatalf("Causes: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("Causes returned %d events, want 2 (tick → initiative)", len(chain))
	}
	if chain[0].ID != tick.ID {
		t.Errorf("chain[0]=%s want tick %s (root-first ordering)", chain[0].ID, tick.ID)
	}
	if chain[1].ID != initiative.ID {
		t.Errorf("chain[1]=%s want initiative %s", chain[1].ID, initiative.ID)
	}
}

// TestCauses_DeepChainOrdered confirms the walk handles arbitrary depth and
// returns oldest-first, not just the 2-hop Pulse star.
func TestCauses_DeepChainOrdered(t *testing.T) {
	j := newTestJournal(t, 0)
	root := appendProv(t, j, event.KindPulseTick, "c-root", "")
	mid := appendProv(t, j, event.KindObserverDelta, "c-mid", root.ID)
	leaf := appendProv(t, j, event.KindSalienceScored, "c-leaf", mid.ID)

	chain, err := j.Causes(leaf.ID)
	if err != nil {
		t.Fatalf("Causes: %v", err)
	}
	gotIDs := []string{}
	for _, e := range chain {
		gotIDs = append(gotIDs, e.ID)
	}
	want := []string{root.ID, mid.ID, leaf.ID}
	if len(gotIDs) != len(want) {
		t.Fatalf("chain len=%d %v, want 3 %v", len(gotIDs), gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("chain[%d]=%s want %s", i, gotIDs[i], want[i])
		}
	}
}

// TestCauses_Terminates asserts the walk always terminates within a deadline.
// A true causation cycle cannot be constructed through Append — causation_id
// must reference an already-appended (older) event, so the graph is acyclic by
// construction — which is exactly why the seen-set guard in Causes is purely
// defensive (against a corrupt or hand-edited journal). This test pins the
// termination + correctness contract on a realistic linear chain; the guard
// itself is belt-and-suspenders that can never trip on runtime-emitted data.
func TestCauses_Terminates(t *testing.T) {
	j := newTestJournal(t, 0)
	root := appendProv(t, j, event.KindPulseTick, "d-root", "")
	childA := appendProv(t, j, event.KindObserverDelta, "d-a", root.ID)
	grand := appendProv(t, j, event.KindInitiativeTaken, "d-g", childA.ID)

	done := make(chan int, 1)
	go func() {
		chain, _ := j.Causes(grand.ID)
		done <- len(chain)
	}()
	select {
	case n := <-done:
		if n != 3 {
			t.Errorf("chain len=%d, want 3 (root → childA → grand)", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Causes did not terminate within 2s")
	}
}

// TestCauses_DanglingParentStopsAtSelf: an event whose causation_id points at an
// absent parent (e.g. a windowed/partial journal) yields just the event itself,
// never an error or an infinite wait.
func TestCauses_DanglingParentStopsAtSelf(t *testing.T) {
	j := newTestJournal(t, 0)
	ev := appendProv(t, j, event.KindInitiativeTaken, "e-1", "no-such-parent-id")
	chain, err := j.Causes(ev.ID)
	if err != nil {
		t.Fatalf("Causes: %v", err)
	}
	if len(chain) != 1 || chain[0].ID != ev.ID {
		t.Fatalf("dangling parent: got %d events, want just the event itself", len(chain))
	}
}

// TestCauses_NotFound: an unknown id is a clean error, mirroring Why.
func TestCauses_NotFound(t *testing.T) {
	j := newTestJournal(t, 0)
	if _, err := j.Causes("does-not-exist"); err == nil {
		t.Fatal("Causes on an unknown id should error")
	}
}

// TestWhy_SingletonNoCorrelation: an event without a correlation is returned
// alone, and an unknown id errors.
func TestWhy_SingletonNoCorrelation(t *testing.T) {
	j := newTestJournal(t, 0)
	solo := appendProv(t, j, event.KindPulseTick, "", "")
	got, err := j.Why(solo.ID)
	if err != nil {
		t.Fatalf("Why: %v", err)
	}
	if len(got) != 1 || got[0].ID != solo.ID {
		t.Fatalf("Why(singleton) returned %d events, want just the event itself", len(got))
	}
	if _, err := j.Why("does-not-exist"); err == nil {
		t.Fatal("Why on an unknown id should error")
	}
}

// TestParentOf_ResolvesSpawnLink: ParentOf walks child→parent via the
// subagent.spawned payload; unknown/empty correlations return "" (M42).
func TestParentOf_ResolvesSpawnLink(t *testing.T) {
	j := newTestJournal(t, 0)
	if _, err := j.Append(event.Spec{
		Subject: "agent.sub.spawn", Kind: event.KindSubAgentSpawned, Actor: "subagent-c1",
		CorrelationID: "p1",
		Payload:       map[string]any{"child_correlation": "c1", "parent": "p1", "task": "x", "depth": 1},
	}); err != nil {
		t.Fatal(err)
	}
	if got := j.ParentOf("c1"); got != "p1" {
		t.Errorf("ParentOf(c1) = %q want p1", got)
	}
	if got := j.ParentOf("unknown"); got != "" {
		t.Errorf("ParentOf(unknown) = %q want empty", got)
	}
	if got := j.ParentOf(""); got != "" {
		t.Errorf("ParentOf(\"\") = %q want empty", got)
	}
}
