// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeEmbedder maps texts onto a 2-dim space by topic: anything mentioning a
// car (in any language) lands on one axis, everything else on the other —
// giving the true synonym semantics the local feature-hash embedder cannot.
type fakeEmbedder struct {
	batches [][]string
	fail    bool
}

func (f *fakeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	f.batches = append(f.batches, append([]string{}, texts...))
	if f.fail {
		return nil, errors.New("embedder upstream down")
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		lt := strings.ToLower(t)
		if strings.Contains(lt, "car") || strings.Contains(lt, "araba") {
			out[i] = []float32{1, 0}
		} else {
			out[i] = []float32{0, 1}
		}
	}
	return out, nil
}

func newEmbedTestManager(t *testing.T) *Manager {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewManager(st, nil)
}

// TestRecall_ProviderEmbedderFindsSynonyms (M884): with a provider embedder
// installed, a recall matches on MEANING — "car" finds the Turkish "araba"
// record, which neither keyword overlap nor the local feature-hash embedding
// can do.
func TestRecall_ProviderEmbedderFindsSynonyms(t *testing.T) {
	m := newEmbedTestManager(t)
	if _, _, err := m.Remember("", RememberSpec{Subject: "vehicle", Content: "sahibinin arabası kırmızı bir araba"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if _, _, err := m.Remember("", RememberSpec{Subject: "deploy", Content: "production deploys happen on fridays"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Baseline: locally, "car" finds nothing (no shared keyword, no shared
	// character n-grams above the noise floor with the deploy record winning).
	local, err := m.RecallScoped("", "car", 5, "")
	if err != nil {
		t.Fatalf("RecallScoped(local): %v", err)
	}
	for _, h := range local {
		if h.Record.Subject == "vehicle" {
			t.Fatalf("precondition broken: local recall already finds the synonym record (score %v)", h.Score)
		}
	}

	m.SetEmbedder(&fakeEmbedder{})
	hits, err := m.RecallScoped("", "car", 5, "")
	if err != nil {
		t.Fatalf("RecallScoped(provider): %v", err)
	}
	if len(hits) == 0 || hits[0].Record.Subject != "vehicle" {
		t.Fatalf("provider recall hits = %+v, want the vehicle record first", hits)
	}
}

// TestRecall_EmbedderFailureFallsBackToLocal (M884): a broken embedder must
// never fail (or empty out) recall — the local hybrid result is returned.
func TestRecall_EmbedderFailureFallsBackToLocal(t *testing.T) {
	m := newEmbedTestManager(t)
	if _, _, err := m.Remember("", RememberSpec{Subject: "k8s", Content: "kubernetes cluster runs on hetzner"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	m.SetEmbedder(&fakeEmbedder{fail: true})

	hits, err := m.RecallScoped("", "kubernetes", 5, "")
	if err != nil {
		t.Fatalf("RecallScoped: %v", err)
	}
	if len(hits) != 1 || hits[0].Record.Subject != "k8s" {
		t.Fatalf("hits = %+v, want the local keyword match despite the embedder failure", hits)
	}
}

// TestRecall_EmbedderCacheBatchesOnce (M884): records are embedded once —
// content-addressing makes the vector cache immutable — so the second recall
// only embeds the query.
func TestRecall_EmbedderCacheBatchesOnce(t *testing.T) {
	m := newEmbedTestManager(t)
	if _, _, err := m.Remember("", RememberSpec{Subject: "a", Content: "alpha fact"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if _, _, err := m.Remember("", RememberSpec{Subject: "b", Content: "beta fact"}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	emb := &fakeEmbedder{}
	m.SetEmbedder(emb)

	if _, err := m.RecallScoped("", "first query", 5, ""); err != nil {
		t.Fatalf("RecallScoped 1: %v", err)
	}
	if _, err := m.RecallScoped("", "second query", 5, ""); err != nil {
		t.Fatalf("RecallScoped 2: %v", err)
	}

	if len(emb.batches) != 2 {
		t.Fatalf("EmbedBatch calls = %d, want 2 (one per recall)", len(emb.batches))
	}
	if got := len(emb.batches[0]); got != 3 {
		t.Errorf("first batch = %d texts, want 3 (query + 2 uncached records)", got)
	}
	if got := len(emb.batches[1]); got != 1 {
		t.Errorf("second batch = %d texts, want 1 (query only — records cached)", got)
	}
}
