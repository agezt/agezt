// SPDX-License-Identifier: MIT

package memory

import (
	"strings"
	"testing"
)

// rememberScoped stores a record tagged with the given scope (empty = shared),
// mirroring what the memory tool does for an agent-provided scope.
func rememberScoped(t *testing.T, m *Manager, subject, content, scope string) {
	t.Helper()
	tags := map[string]string{"source": "agent"}
	if scope != "" {
		tags["scope"] = scope
	}
	if _, _, err := m.Remember("corr", RememberSpec{Type: TypeFact, Subject: subject, Content: content, Tags: tags}); err != nil {
		t.Fatalf("Remember(%q): %v", scope, err)
	}
}

func recallSubjects(t *testing.T, m *Manager, query, scope string) map[string]bool {
	t.Helper()
	hits, err := m.RecallScoped("corr", query, 50, scope)
	if err != nil {
		t.Fatalf("RecallScoped(%q): %v", scope, err)
	}
	got := map[string]bool{}
	for _, h := range hits {
		got[h.Record.Subject] = true
	}
	return got
}

// TestRecallScoped_VisibilityRules is the core contract: shared records are
// always visible; a scope's private record is visible only to that scope; and an
// empty-scope (shared / auto-recall) view never sees any private record.
func TestRecallScoped_VisibilityRules(t *testing.T) {
	m, _ := newTestManager(t)
	// All three share the queryable term "project" so Search returns every record;
	// only the scope filter decides visibility.
	rememberScoped(t, m, "deploy-url", "project deploy URL is example.com", "")            // shared
	rememberScoped(t, m, "researcher-draft", "project research draft notes", "researcher") // private
	rememberScoped(t, m, "writer-style", "project writing style preference", "writer")     // private

	// The researcher sees shared + its own, never the writer's.
	r := recallSubjects(t, m, "project", "researcher")
	if !r["deploy-url"] || !r["researcher-draft"] {
		t.Errorf("researcher should see shared + own scope, got %v", r)
	}
	if r["writer-style"] {
		t.Errorf("researcher must NOT see the writer's private note, got %v", r)
	}

	// The writer sees shared + its own, never the researcher's.
	w := recallSubjects(t, m, "project", "writer")
	if !w["deploy-url"] || !w["writer-style"] {
		t.Errorf("writer should see shared + own scope, got %v", w)
	}
	if w["researcher-draft"] {
		t.Errorf("writer must NOT see the researcher's private note, got %v", w)
	}

	// The shared/auto-recall view (empty scope) sees ONLY shared records.
	s := recallSubjects(t, m, "project", "")
	if !s["deploy-url"] {
		t.Errorf("shared view should see shared records, got %v", s)
	}
	if s["researcher-draft"] || s["writer-style"] {
		t.Errorf("shared view must NOT see any private note, got %v", s)
	}
}

// TestRecall_DefaultIsSharedOnly: the legacy Recall (no scope) behaves like the
// shared view, so the daemon's automatic pre-run recall can never leak private
// notes into an unrelated run.
func TestRecall_DefaultIsSharedOnly(t *testing.T) {
	m, _ := newTestManager(t)
	rememberScoped(t, m, "shared-fact", "project everyone can see this", "")
	rememberScoped(t, m, "secret", "project only mine", "agent-x")

	hits, err := m.Recall("corr", "project", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Record.Subject == "secret" {
			t.Fatal("Recall (no scope) leaked a private record")
		}
	}
}

// TestToolInputTags_Scope: the tool maps an input scope onto a scope tag, and
// omits it when absent.
func TestToolInputTags_Scope(t *testing.T) {
	if tags := (toolInput{}).Tags(); tags["scope"] != "" {
		t.Errorf("no scope → no scope tag, got %v", tags)
	}
	tags := (toolInput{Scope: " researcher "}).Tags()
	if tags["scope"] != "researcher" {
		t.Errorf("scope tag = %q, want researcher (trimmed)", tags["scope"])
	}
	if tags["source"] != "agent" {
		t.Errorf("source tag should still be agent, got %v", tags)
	}
}

// TestRenderHits_AnnotatesScope: a recalled private record is labelled so the
// agent can tell shared from scoped knowledge.
func TestRenderHits_AnnotatesScope(t *testing.T) {
	hits := []Scored{
		{Record: Record{Type: TypeFact, Subject: "s", Content: "c", Tags: map[string]string{"scope": "researcher"}}},
	}
	if out := renderHits(hits); !strings.Contains(out, "scope: researcher") {
		t.Errorf("render should annotate the scope, got %q", out)
	}
}
