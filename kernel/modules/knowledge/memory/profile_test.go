// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"strings"
	"testing"
	"time"
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

// TestDistillProfile_SupersedesTheChangedFacet proves the profile-facet
// supersede bug.
//
// The facet's subject is stable ("operator profile: communication style"), so
// a facet is meant to be UPDATED across passes, never duplicated. But a
// memory record's identity is content-addressed
// (ScopedID(type, subject, content, scope)), so a pass whose synthesized
// content CHANGES writes a second record under the same subject — and the
// previous one is never superseded. The operator then gets two conflicting
// versions of the same facet injected into every run.
//
// The rerun test above only proves the same-content case, where the
// content-addressed id happens to be identical.
func TestDistillProfile_SupersedesTheChangedFacet(t *testing.T) {
	m := newConsolidationManager(t)
	seedProfileCorpus(t, m)
	first := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers terse, direct replies."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c1", &first, "test-model"); err != nil {
		t.Fatal(err)
	}
	second := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers long, detailed replies."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c2", &second, "test-model"); err != nil {
		t.Fatal(err)
	}
	recs := profileRecords(t, m)
	if len(recs) != 1 {
		t.Fatalf("the profile carries %d records for one facet, want 1 (the changed facet supersedes the old one): %+v", len(recs), recs)
	}
	if recs[0].Content != "Prefers long, detailed replies." {
		t.Errorf("the surviving facet = %q, want the newer content", recs[0].Content)
	}
	// The replaced facet must be marked superseded, not silently dropped: the
	// audit trail has to show which record the new one replaced — and it has to
	// point at the record that replaced it.
	all, err := m.store.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var superseded int
	for _, r := range all {
		if r.SupersededBy == "" {
			continue
		}
		superseded++
		if r.Subject != profileSubjectPrefix+"communication style" {
			t.Errorf("an unrelated record was superseded: %+v", r)
		}
		if r.SupersededBy != recs[0].ID {
			t.Errorf("the retired facet points at %q, want the surviving facet %q", r.SupersededBy, recs[0].ID)
		}
	}
	if superseded != 1 {
		t.Errorf("superseded facet records = %d, want 1", superseded)
	}
}

// TestDistillProfile_SupersedesOnlyTheChangedFacet: a pass that changes ONE
// facet must not retire the others. The profile is a set of facets, and each
// one is updated on its own.
func TestDistillProfile_SupersedesOnlyTheChangedFacet(t *testing.T) {
	m := newConsolidationManager(t)
	seedProfileCorpus(t, m)
	first := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers terse replies."},` +
			`{"facet":"expertise","content":"Go backends."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c1", &first, "test-model"); err != nil {
		t.Fatal(err)
	}
	second := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers long replies."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c2", &second, "test-model"); err != nil {
		t.Fatal(err)
	}
	recs := profileRecords(t, m)
	if len(recs) != 2 {
		t.Fatalf("the profile carries %d facets, want 2 (the changed one and the untouched one): %+v", len(recs), recs)
	}
	var sawExpertise bool
	for _, r := range recs {
		if strings.HasSuffix(r.Subject, "expertise") {
			sawExpertise = true
			if r.Content != "Go backends." {
				t.Errorf("the untouched facet changed: %q", r.Content)
			}
		}
	}
	if !sawExpertise {
		t.Errorf("the untouched facet was retired: %+v", recs)
	}
}

// TestDistillProfile_KeepsAPrivateFacetOfTheSameName: the profile is the
// operator's global picture, so a facet is written in SHARED scope. A record
// with the same subject in a private scope belongs to another scope and must
// not be retired by the shared write.
func TestDistillProfile_KeepsAPrivateFacetOfTheSameName(t *testing.T) {
	m := newConsolidationManager(t)
	seedProfileCorpus(t, m)
	private := Record{
		ID:         "private-facet",
		Type:       TypePreference,
		Subject:    profileSubjectPrefix + "communication style",
		Content:    "A private note about the operator.",
		Tags:       map[string]string{"scope": "researcher", "source": "profile"},
		CreatedMS:  1000,
		LastSeenMS: 1000,
		HalfLifeMS: 15552000000,
	}
	if err := m.store.Put(private); err != nil {
		t.Fatalf("Put: %v", err)
	}
	prov := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers terse, direct replies."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c1", &prov, "test-model"); err != nil {
		t.Fatal(err)
	}
	rec, found, err := m.store.Get("private-facet")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("the private record is gone")
	}
	if rec.SupersededBy != "" {
		t.Errorf("the private record was retired by the shared write: %+v", rec)
	}
}

// TestDistillProfile_ToleratesARecordThatVanishedConcurrently: the facet the
// pass is about to retire may be deleted between the pass's read and its write
// (another writer, a hygiene pass, a compaction). The pass must retire what it
// can and carry on — the vanished record needs no superseding.
func TestDistillProfile_ToleratesARecordThatVanishedConcurrently(t *testing.T) {
	realStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m := NewManager(realStore, nil)
	m.now = func() time.Time { return time.UnixMilli(1000) }
	seedProfileCorpus(t, m)
	first := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers terse, direct replies."}]}`,
	}}
	if _, err := m.DistillProfile(context.Background(), "c1", &first, "test-model"); err != nil {
		t.Fatal(err)
	}
	recs := profileRecords(t, m)
	if len(recs) != 1 {
		t.Fatalf("the profile carries %d facets, want 1", len(recs))
	}
	// The facet record vanishes from the store's point of view, but the pass
	// still sees it in the snapshot it read.
	store := &absentStore{real: realStore, absentID: recs[0].ID, absentGet: true}
	m2 := NewManager(store, nil)
	m2.now = func() time.Time { return time.UnixMilli(1000) }
	second := scriptedProvider{answers: []string{
		`{"facets":[{"facet":"communication style","content":"Prefers long, detailed replies."}]}`,
	}}
	report, err := m2.DistillProfile(context.Background(), "c2", &second, "test-model")
	if err != nil {
		t.Fatalf("a record that vanished concurrently must not abort the pass: %v", err)
	}
	if report.FacetsWritten != 1 {
		t.Errorf("facets written = %d, want 1", report.FacetsWritten)
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
