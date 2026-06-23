// SPDX-License-Identifier: MIT

package controlplane

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/memory"
)

// memorySuggestions ranks high-signal records, dedupes by subject, caps at max,
// and phrases each by type.
func TestMemorySuggestions_RankDedupeCap(t *testing.T) {
	recs := []memory.Record{
		{Type: memory.TypeObservation, Subject: "noise", Content: "ignore me", Confidence: 1, LastSeenMS: 999},
		{Type: memory.TypeFact, Subject: "deploy", Content: "prod is eu-west-1", Confidence: 0.6, LastSeenMS: 10},
		{Type: memory.TypePreference, Subject: "style", Content: "be blunt", Confidence: 0.9, LastSeenMS: 5},
		// Duplicate subject (case-insensitive) — should be dropped after the first.
		{Type: memory.TypeFact, Subject: "Deploy", Content: "older note", Confidence: 0.5, LastSeenMS: 1},
		{Type: memory.TypeSummary, Subject: "refactor", Content: "split the governor", Confidence: 0.7, LastSeenMS: 50},
		{Type: memory.TypeFact, Subject: "", Content: "no subject", Confidence: 1, LastSeenMS: 100}, // skipped
	}

	got := memorySuggestions(recs, 3)
	if len(got) != 3 {
		t.Fatalf("want 3 suggestions, got %d: %+v", len(got), got)
	}
	// Highest confidence first: style (0.9), refactor (0.7), deploy (0.6).
	wantSubjects := []string{"style", "refactor", "deploy"}
	for i, want := range wantSubjects {
		if !strings.Contains(strings.ToLower(got[i].Label), want) {
			t.Errorf("suggestion %d label %q does not mention %q", i, got[i].Label, want)
		}
		if got[i].Category != "memory" || got[i].Icon != "brain" {
			t.Errorf("suggestion %d not tagged memory/brain: %+v", i, got[i])
		}
	}
	// OBSERVATION is excluded and the duplicate "Deploy" must not reappear.
	for _, s := range got {
		if strings.Contains(strings.ToLower(s.Label), "noise") {
			t.Error("OBSERVATION record leaked into suggestions")
		}
	}
}

// Each record type produces a distinctly-phrased, non-empty prompt.
func TestMemorySuggestion_PhrasingByType(t *testing.T) {
	cases := []struct {
		typ      memory.Type
		wantWord string
	}{
		{memory.TypePreference, "preference"},
		{memory.TypeSummary, "continue"},
		{memory.TypeFact, "know about"},
		{memory.TypeRelation, "know about"},
	}
	for _, c := range cases {
		s := memorySuggestion(memory.Record{Type: c.typ, Subject: "X", Content: "details here"})
		if s.Prompt == "" || s.Label == "" {
			t.Fatalf("%s produced empty label/prompt", c.typ)
		}
		if !strings.Contains(strings.ToLower(s.Prompt), c.wantWord) {
			t.Errorf("%s prompt %q missing %q", c.typ, s.Prompt, c.wantWord)
		}
	}
}

// snip collapses newlines and truncates with an ellipsis past the limit.
func TestSnip(t *testing.T) {
	if got := snip("a\nb", 10); got != "a b" {
		t.Errorf("snip newline = %q want %q", got, "a b")
	}
	long := strings.Repeat("x", 200)
	got := snip(long, 10)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > 11 {
		t.Errorf("snip truncation = %q", got)
	}
}

// appendUnique skips duplicate IDs and respects the cap.
func TestAppendUnique(t *testing.T) {
	base := []ChatSuggestion{{ID: "a"}, {ID: "b"}}
	extras := []ChatSuggestion{{ID: "b"}, {ID: "c"}, {ID: "d"}}
	got := appendUnique(base, extras, 3)
	if len(got) != 3 {
		t.Fatalf("want 3, got %d: %+v", len(got), got)
	}
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	want := []string{"a", "b", "c"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("appendUnique[%d] = %q want %q", i, ids[i], want[i])
		}
	}
}

// buildSuggestions still returns tool-context sets (unchanged behavior).
func TestBuildSuggestions_ToolContext(t *testing.T) {
	if got := buildSuggestions("", nil); len(got) != 4 {
		t.Errorf("no-context default = %d suggestions, want 4", len(got))
	}
	got := buildSuggestions("", []string{"write"})
	if len(got) == 0 {
		t.Fatal("expected tool-context suggestions for write")
	}
	for _, s := range got {
		if s.ID == "" {
			t.Error("suggestion missing ID")
		}
	}
}
