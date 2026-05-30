// SPDX-License-Identifier: MIT

package worldmodel

import "testing"

const day = 24 * 60 * 60 * 1000

func ent(id, name string, aliases []string, weight float64, lastSeen int64) Entity {
	return Entity{ID: id, Kind: KindProject, Name: name, Aliases: aliases, Weight: weight, CreatedMS: lastSeen, LastSeenMS: lastSeen}
}

func TestResolveExactAndAlias(t *testing.T) {
	now := int64(10 * day)
	es := []Entity{
		ent("lic", "Lictor", []string{"portfolio", "the repos"}, 1, now),
		ent("oth", "Other", nil, 1, now),
	}
	// Alias phrase resolves to Lictor.
	hits := Resolve(es, "the portfolio", 5, now)
	if len(hits) == 0 || hits[0].Entity.ID != "lic" {
		t.Fatalf("alias resolve failed: %+v", hits)
	}
	// Exact name beats partial token overlap.
	hits = Resolve(es, "Lictor", 5, now)
	if hits[0].Entity.ID != "lic" {
		t.Fatalf("exact name resolve failed: %+v", hits)
	}
}

func TestResolveExcludesTombstonedAndSuperseded(t *testing.T) {
	now := int64(day)
	dead := ent("d", "Ghost", nil, 1, now)
	dead.Tombstoned = true
	sup := ent("s", "Ghost", nil, 1, now)
	sup.SupersededBy = "x"
	es := []Entity{dead, sup}
	if hits := Resolve(es, "Ghost", 5, now); len(hits) != 0 {
		t.Fatalf("inactive entities must not resolve, got %+v", hits)
	}
}

func TestResolveWeightAndRecencyRank(t *testing.T) {
	now := int64(100 * day)
	// Same token match; higher weight + more recent should rank first.
	hot := ent("hot", "alpha project", nil, 1.0, now)
	cold := ent("cold", "alpha project", nil, 0.2, now-50*day)
	hits := Resolve([]Entity{cold, hot}, "alpha", 5, now)
	if len(hits) != 2 || hits[0].Entity.ID != "hot" {
		t.Fatalf("weight/recency ranking wrong: %+v", hits)
	}
}

func TestResolveEmptyPhrase(t *testing.T) {
	es := []Entity{ent("a", "A", nil, 1, 0)}
	if hits := Resolve(es, "   ", 5, 0); len(hits) != 0 {
		t.Errorf("empty phrase should resolve to nothing, got %+v", hits)
	}
}

func TestNeighbors(t *testing.T) {
	es := []Entity{
		ent("lic", "Lictor", nil, 1, 0),
		ent("go", "go-stdlib", nil, 1, 0),
		ent("ersin", "Ersin", nil, 1, 0),
	}
	rs := []Relation{
		{ID: "r1", From: "lic", Verb: VerbDependsOn, To: "go", CreatedMS: 1, LastSeenMS: 1},
		{ID: "r2", From: "ersin", Verb: VerbOwns, To: "lic", CreatedMS: 2, LastSeenMS: 2},
		{ID: "dead", From: "lic", Verb: VerbRelatesTo, To: "go", CreatedMS: 3, Tombstoned: true},
	}
	ns := Neighbors("lic", es, rs)
	if len(ns) != 2 {
		t.Fatalf("expected 2 active neighbors, got %d: %+v", len(ns), ns)
	}
	// r1 outgoing (lic depends_on go); r2 incoming (ersin owns lic). Sorted by CreatedMS.
	if !ns[0].Outgoing || ns[0].Other.ID != "go" {
		t.Errorf("first neighbor wrong: %+v", ns[0])
	}
	if ns[1].Outgoing || ns[1].Other.ID != "ersin" {
		t.Errorf("second neighbor wrong: %+v", ns[1])
	}
}
