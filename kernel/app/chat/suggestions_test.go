// SPDX-License-Identifier: MIT

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/platform/schema"
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
	base := []Suggestion{{ID: "a"}, {ID: "b"}}
	extras := []Suggestion{{ID: "b"}, {ID: "c"}, {ID: "d"}}
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

func suggestionIDs(t *testing.T, s *Service, in SuggestionsRequest) string {
	t.Helper()
	out, err := s.Suggestions(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(out.Suggestions))
	for _, x := range out.Suggestions {
		ids = append(ids, x.ID)
	}
	return strings.Join(ids, ",")
}

// Suggestions decodes the session id and tool names like the legacy codec,
// leads with memory-derived chips and fills from the tool-context catalog.
func TestSuggestions(t *testing.T) {
	bare := New(Ports{})
	for _, raw := range []string{`5`, `null`, `true`, `[]`, `{}`} {
		if _, err := bare.Suggestions(context.Background(), SuggestionsRequest{SessionID: json.RawMessage(raw)}); err == nil || err.Error() != "args.session_id must be a string" {
			t.Fatal(raw, err)
		}
	}
	defaults := "debug-why,debug-root-cause,debug-alternatives,explore-deepen"
	for raw, want := range map[string]string{
		``:                         defaults,
		`""`:                       defaults,
		`" , ,"`:                   defaults,
		`5`:                        defaults,
		`null`:                     defaults,
		`{"write":true}`:           defaults,
		`[1,null," "]`:             defaults,
		`"Write, bash"`:            "modify-test,review-security,modify-refactor,debug-why",
		`[" GIT ",7]`:              "review-best-practices,workflow-next-steps",
		`"web_fetch"`:              "explore-deepen,explore-tradeoffs",
		`"unknown"`:                "explore-deepen,modify-implement,workflow-next-steps,review-best-practices",
		`["write","edit","patch"]`: "modify-test,review-security,modify-refactor",
	} {
		if got := suggestionIDs(t, bare, SuggestionsRequest{SessionID: json.RawMessage(`"s"`), Tools: json.RawMessage(raw)}); got != want {
			t.Fatalf("tools %s: %s, want %s", raw, got, want)
		}
	}
	recs := []memory.Record{
		{Type: memory.TypeFact, Subject: "deploy", Confidence: 0.6},
		{Type: memory.TypePreference, Subject: "style", Confidence: 0.9},
		{Type: memory.TypeSummary, Subject: "debug why", Confidence: 0.7},
		{Type: memory.TypeFact, Subject: "extra", Confidence: 0.1},
	}
	withMemory := New(Ports{ActiveMemory: func() ([]memory.Record, error) { return recs, nil }})
	if got := suggestionIDs(t, withMemory, SuggestionsRequest{}); got != "mem-style,mem-debug-why,mem-deploy,debug-why,debug-root-cause" {
		t.Fatal(got)
	}
	failing := New(Ports{ActiveMemory: func() ([]memory.Record, error) { return recs, errors.New("boom") }})
	if got := suggestionIDs(t, failing, SuggestionsRequest{}); got != defaults {
		t.Fatal("a memory read error falls back to the catalog", got)
	}
	out, _ := bare.Suggestions(context.Background(), SuggestionsRequest{})
	raw, _ := json.Marshal(out)
	if !strings.HasPrefix(string(raw), `{"suggestions":[{"id":"debug-why","label":"Why did this happen?","prompt":"Explain why this result occurred in detail","category":"debug","icon":"search"},`) {
		t.Fatal(string(raw))
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("provider is required")
	}
	ops, err := Operations(func(context.Context) *Service { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(len(ops), err)
	}
	s := ops[0].Spec()
	out, err := schema.FromType(reflect.TypeFor[SuggestionsOutput](), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "chat_suggestions" || !s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "GET" || s.HTTP.Path != "/api/suggestions" || s.Input != reflect.TypeFor[SuggestionsRequest]() || s.Output != reflect.TypeFor[SuggestionsOutput]() || string(s.OutputSchema) != string(out) {
		t.Fatalf("spec: %+v", s)
	}
	for _, in := range []string{`{}`, `{"session_id":null,"tools":5,"tenant":"t"}`} {
		if err := schema.ValidateJSON(s.InputSchema, json.RawMessage(in)); err != nil {
			t.Fatal(in, err)
		}
	}
	if !strings.Contains(string(s.InputSchema), `"properties":{"session_id":{},"tools":{}}`) {
		t.Fatal(string(s.InputSchema))
	}
}
