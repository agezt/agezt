// SPDX-License-Identifier: MIT

package worldmodel

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func TestDecayLowersStaleLeavesFresh(t *testing.T) {
	g, j := newTestGraph(t)
	nowMS := fixedNow.UnixMilli()

	// Fresh: seen now. Stale: seen 30 days ago.
	fresh, _, _ := g.Upsert("seed", UpsertSpec{Kind: KindProject, Name: "fresh", Weight: 1.0})
	stale := Entity{
		ID: EntityID(KindProject, "stale"), Kind: KindProject, Name: "stale",
		Weight: 1.0, CreatedMS: nowMS - 40*day, LastSeenMS: nowMS - 30*day,
	}
	if err := g.store.PutEntity(stale); err != nil {
		t.Fatal(err)
	}

	n, err := g.Decay("refl-1", DecayOptions{StaleAfterMS: 14 * day, Factor: 0.5, Floor: 0.1})
	if err != nil {
		t.Fatalf("decay: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 entity decayed, got %d", n)
	}
	gotStale, _, _ := g.Get(stale.ID)
	if gotStale.Weight != 0.5 {
		t.Errorf("stale weight should be 0.5, got %v", gotStale.Weight)
	}
	gotFresh, _, _ := g.Get(fresh.ID)
	if gotFresh.Weight != 1.0 {
		t.Errorf("fresh weight must be untouched, got %v", gotFresh.Weight)
	}
	// fresh Upsert journals 1 entity.upserted; the decay journals 1 more
	// (action=decay). The stale entity was Put directly, bypassing the bus.
	if countKind(t, j, event.KindWorldEntityUpserted) != 2 {
		t.Errorf("expected 2 entity.upserted events (1 upsert + 1 decay), got %d",
			countKind(t, j, event.KindWorldEntityUpserted))
	}
}

func TestDecayFloorAndIdempotent(t *testing.T) {
	g, _ := newTestGraph(t)
	nowMS := fixedNow.UnixMilli()
	e := Entity{
		ID: EntityID(KindTopic, "x"), Kind: KindTopic, Name: "x",
		Weight: 0.15, CreatedMS: nowMS - 40*day, LastSeenMS: nowMS - 30*day,
	}
	_ = g.store.PutEntity(e)

	// First pass: 0.15*0.5=0.075 < floor 0.1 → clamped to 0.1.
	n, _ := g.Decay("r", DecayOptions{StaleAfterMS: day, Factor: 0.5, Floor: 0.1})
	if n != 1 {
		t.Fatalf("expected 1 decayed, got %d", n)
	}
	got, _, _ := g.Get(e.ID)
	if got.Weight != 0.1 {
		t.Errorf("weight should floor at 0.1, got %v", got.Weight)
	}
	// Second pass: already at floor → no further change, not counted.
	n2, _ := g.Decay("r", DecayOptions{StaleAfterMS: day, Factor: 0.5, Floor: 0.1})
	if n2 != 0 {
		t.Errorf("at-floor entity should not decay again, got %d", n2)
	}
}

func TestDecayDefaults(t *testing.T) {
	g, _ := newTestGraph(t)
	// All-zero opts → defaults; a brand-new entity is fresh → nothing decays.
	_, _, _ = g.Upsert("s", UpsertSpec{Kind: KindProject, Name: "p"})
	n, err := g.Decay("r", DecayOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("fresh entity under default staleness should not decay, got %d", n)
	}
}
