// SPDX-License-Identifier: MIT

package worldmodel

import "testing"

// Entity provenance is first-writer-wins: Upsert sets SourceEvent only on first
// creation (`if ev != nil && e.SourceEvent == ""`), so re-observing an entity must
// PRESERVE the id of the event that originally created it — that is the meaningful
// provenance for audit/causation ("why does the world model know about X?").
// TestUpsertCreatesAndJournals only checks that a created entity carries provenance,
// not that re-observation preserves it, so mutation testing (M503) showed the `&&`
// could flip to `||` (overwriting provenance with the latest mention, last-writer)
// undetected. Pin the preservation.
func TestUpsert_PreservesOriginalProvenanceOnReObserve(t *testing.T) {
	g, _ := newTestGraph(t)

	e1, created, err := g.Upsert("corr-1", UpsertSpec{Kind: KindProject, Name: "Lictor"})
	if err != nil || !created {
		t.Fatalf("first upsert: created=%v err=%v", created, err)
	}
	if e1.SourceEvent == "" {
		t.Fatal("first upsert must set provenance")
	}

	// Re-observe the same entity (same Kind+Name) under a different correlation.
	e2, created2, err := g.Upsert("corr-2", UpsertSpec{Kind: KindProject, Name: "Lictor"})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if created2 {
		t.Fatal("re-observing the same entity must not create a new one")
	}
	if e2.SourceEvent != e1.SourceEvent {
		t.Errorf("re-observe overwrote provenance: got %q, want the original %q (first-writer-wins)", e2.SourceEvent, e1.SourceEvent)
	}
}
