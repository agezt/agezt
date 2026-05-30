// SPDX-License-Identifier: MIT

package memory

import (
	"path/filepath"
	"testing"
)

func TestContentIDDedupAndDistinct(t *testing.T) {
	a := ContentID(TypeFact, "lictor", "Agezt is a Go agentic OS")
	b := ContentID(TypeFact, "lictor", "Agezt is a Go agentic OS")
	if a != b {
		t.Fatalf("identical content must produce identical id: %s != %s", a, b)
	}
	// NUL separation: ("ab","c") must not collide with ("a","bc").
	if ContentID(TypeFact, "ab", "c") == ContentID(TypeFact, "a", "bc") {
		t.Fatal("subject/content boundary collision")
	}
	if ContentID(TypeFact, "x", "y") == ContentID(TypeSummary, "x", "y") {
		t.Fatal("type must participate in the address")
	}
}

func TestFileStorePutGetAllPersist(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := Record{ID: "id1", Type: TypeFact, Subject: "s", Content: "c", Confidence: 1, CreatedMS: 10, LastSeenMS: 10}
	if err := s.Put(rec); err != nil {
		t.Fatal(err)
	}
	if s.Count() != 1 {
		t.Fatalf("count = %d, want 1", s.Count())
	}

	// Reopen → record survives (atomic snapshot on disk).
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := s2.Get("id1")
	if err != nil || !ok {
		t.Fatalf("get after reopen: ok=%v err=%v", ok, err)
	}
	if got.Content != "c" {
		t.Fatalf("content = %q", got.Content)
	}
	if _, ok, _ := s2.Get("nope"); ok {
		t.Fatal("absent id should report not found")
	}
}

func TestFileStoreEmptyContentRejected(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := s.Put(Record{ID: "x", Content: "   "}); err != ErrEmptyContent {
		t.Fatalf("want ErrEmptyContent, got %v", err)
	}
}

func TestAllDeterministicOrder(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.Put(Record{ID: "b", Content: "c", CreatedMS: 20})
	_ = s.Put(Record{ID: "a", Content: "c", CreatedMS: 20})
	_ = s.Put(Record{ID: "z", Content: "c", CreatedMS: 5})
	all, _ := s.All()
	// CreatedMS asc, ties by id asc → z(5), a(20), b(20).
	want := []string{"z", "a", "b"}
	for i, w := range want {
		if all[i].ID != w {
			t.Fatalf("order[%d] = %s, want %s", i, all[i].ID, w)
		}
	}
}

func TestSearchRankingAndFilters(t *testing.T) {
	now := int64(1_000_000_000_000)
	day := int64(24 * 60 * 60 * 1000)
	rs := []Record{
		{ID: "fresh", Subject: "agezt kernel", Content: "the agezt kernel journals events", Confidence: 0.9, LastSeenMS: now},
		{ID: "stale", Subject: "agezt", Content: "agezt is old news", Confidence: 0.9, LastSeenMS: now - 30*day},
		{ID: "nomatch", Subject: "weather", Content: "it is raining", Confidence: 1, LastSeenMS: now},
		{ID: "dead", Subject: "agezt", Content: "agezt forgotten", Confidence: 1, LastSeenMS: now, Tombstoned: true},
		{ID: "old", Subject: "agezt", Content: "superseded agezt fact", Confidence: 1, LastSeenMS: now, SupersededBy: "fresh"},
	}
	hits := Search(rs, "agezt kernel", 10, now)

	ids := map[string]bool{}
	for _, h := range hits {
		ids[h.Record.ID] = true
	}
	if ids["nomatch"] {
		t.Fatal("zero-overlap record must be excluded")
	}
	if ids["dead"] {
		t.Fatal("tombstoned record must be excluded")
	}
	if ids["old"] {
		t.Fatal("superseded record must be excluded")
	}
	if len(hits) == 0 || hits[0].Record.ID != "fresh" {
		t.Fatalf("expected 'fresh' (more overlap + recent) ranked first, got %+v", hits)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	rs := []Record{{ID: "a", Subject: "x", Content: "y", LastSeenMS: 1}}
	if got := Search(rs, "   ", 5, 100); len(got) != 0 {
		t.Fatalf("empty query must return no hits, got %d", len(got))
	}
}

func TestSearchLimit(t *testing.T) {
	var rs []Record
	for _, id := range []string{"a", "b", "c", "d"} {
		rs = append(rs, Record{ID: id, Subject: "agezt", Content: "agezt fact", Confidence: 1, LastSeenMS: 5})
	}
	if got := Search(rs, "agezt", 2, 10); len(got) != 2 {
		t.Fatalf("limit=2 returned %d", len(got))
	}
}

func TestOpenCreatesDir(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "a", "b", "memory")
	if _, err := Open(nested); err != nil {
		t.Fatalf("Open should create nested dir: %v", err)
	}
}
