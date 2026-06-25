// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"strings"
	"testing"
)

// seedProfileCorpus writes a handful of shared-scope task facts (the kind the
// per-run distiller produces) plus one private-scope note that must be excluded.
func seedProfileCorpus(t *testing.T, m *Manager) {
	t.Helper()
	shared := []struct{ subj, content string }{
		{"work hours", "the operator works late evenings and prefers terse replies"},
		{"stack", "the operator builds Go backends and React frontends"},
		{"deploy", "deploys go through a self-hosted CI on WSL runners"},
	}
	for _, s := range shared {
		if _, _, err := m.Remember("", RememberSpec{Type: TypeFact, Subject: s.subj, Content: s.content, Force: true}); err != nil {
			t.Fatalf("seed shared: %v", err)
		}
	}
	if _, _, err := m.Remember("", RememberSpec{Type: TypeFact, Subject: "secret", Content: "a private agent note", Tags: map[string]string{"scope": "researcher"}, Force: true}); err != nil {
		t.Fatalf("seed private: %v", err)
	}
}

func profileRecords(t *testing.T, m *Manager) []Record {
	t.Helper()
	active, err := m.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	var out []Record
	for _, r := range active {
		if strings.HasPrefix(r.Subject, profileSubjectPrefix) {
			out = append(out, r)
		}
	}
	return out
}

func TestDistillProfile_SynthesizesFacetsAsSharedPreferences(t *testing.T) {
	m := newConsolidationManager(t)
	seedProfileCorpus(t, m)
	prov := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers terse, direct replies."},` +
			`{"facet":"expertise","content":"Go backends and React frontends."},` +
			`{"facet":"made up","content":"should be dropped — not a valid facet"}]}`,
	}}
	report, err := m.DistillProfile(context.Background(), "corr-p", &prov, "test-model")
	if err != nil {
		t.Fatalf("DistillProfile: %v", err)
	}
	if report.InputRecords != 3 { // 3 shared facts; the private note is excluded
		t.Errorf("input records = %d, want 3 (private scope excluded)", report.InputRecords)
	}
	if report.FacetsWritten != 2 { // the invalid "made up" facet is dropped
		t.Errorf("facets written = %d, want 2", report.FacetsWritten)
	}
	recs := profileRecords(t, m)
	if len(recs) != 2 {
		t.Fatalf("profile records = %d, want 2", len(recs))
	}
	for _, r := range recs {
		if r.Type != TypePreference {
			t.Errorf("facet %q type = %s, want PREFERENCE", r.Subject, r.Type)
		}
		if r.Tags["scope"] != "" {
			t.Errorf("facet %q scope = %q, want shared (empty)", r.Subject, r.Tags["scope"])
		}
		if r.Tags["source"] != "profile" {
			t.Errorf("facet %q source = %q, want profile", r.Subject, r.Tags["source"])
		}
	}
}

func TestDistillProfile_ReinforcesNotDuplicatesOnRerun(t *testing.T) {
	m := newConsolidationManager(t)
	seedProfileCorpus(t, m)
	prov := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers terse, direct replies."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c1", &prov, "test-model"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DistillProfile(context.Background(), "c2", &prov, "test-model"); err != nil {
		t.Fatal(err)
	}
	if recs := profileRecords(t, m); len(recs) != 1 {
		t.Fatalf("rerun produced %d profile records, want 1 (stable subject reinforces)", len(recs))
	}
}

func TestDistillProfile_NoInputIsNoOp(t *testing.T) {
	m := newConsolidationManager(t)
	prov := scriptedProvider{answers: []string{`{"facets":[{"facet":"expertise","content":"x"}]}`}}
	report, err := m.DistillProfile(context.Background(), "c", &prov, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if report.InputRecords != 0 || report.FacetsWritten != 0 {
		t.Errorf("empty store should be a no-op, got %+v", report)
	}
	if prov.calls != 0 {
		t.Errorf("provider called %d times on empty store, want 0", prov.calls)
	}
}

func TestProfileText(t *testing.T) {
	m := newConsolidationManager(t)
	if got := m.ProfileText(); got != "" {
		t.Errorf("empty profile text = %q, want \"\"", got)
	}
	if _, _, err := m.Remember("", RememberSpec{Type: TypePreference, Subject: profileSubjectPrefix + "expertise", Content: "Go and React.", Tags: map[string]string{"scope": "", "source": "profile"}, Force: true}); err != nil {
		t.Fatal(err)
	}
	got := m.ProfileText()
	if !strings.Contains(got, "expertise: Go and React.") {
		t.Errorf("profile text = %q, want it to contain the facet line", got)
	}
	if strings.Contains(got, profileSubjectPrefix) {
		t.Errorf("profile text should strip the subject prefix, got %q", got)
	}
}
