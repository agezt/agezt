// SPDX-License-Identifier: MIT

package memory

import (
	"math"
	"testing"
)

func TestEmbed_DeterministicAndNormalized(t *testing.T) {
	a := Embed("the kubernetes cluster runs in frankfurt")
	b := Embed("the kubernetes cluster runs in frankfurt")
	if len(a) != EmbedDim || len(b) != EmbedDim {
		t.Fatalf("dim = %d/%d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("not deterministic at %d: %v vs %v", i, a[i], b[i])
		}
	}
	var norm float64
	for _, v := range a {
		norm += float64(v) * float64(v)
	}
	if math.Abs(norm-1) > 1e-5 {
		t.Fatalf("not L2-normalized: %v", norm)
	}
	if Embed("") != nil || Embed("a .!") != nil { // no tokens (1-char dropped)
		t.Fatal("empty/noise text must embed to nil")
	}
}

func TestCosine_RelatedVsUnrelated(t *testing.T) {
	base := Embed("the kubernetes cluster needs a node upgrade")
	typo := Embed("kubenetes cluster upgrade")               // misspelled
	morph := Embed("kubernetes clusters upgrades")           // inflected
	other := Embed("pizza dough requires slow fermentation") // unrelated

	if got := Cosine(base, base); math.Abs(got-1) > 1e-5 {
		t.Fatalf("self cosine = %v", got)
	}
	ct, cm, co := Cosine(base, typo), Cosine(base, morph), Cosine(base, other)
	if ct <= co || cm <= co {
		t.Fatalf("related must beat unrelated: typo=%v morph=%v other=%v", ct, cm, co)
	}
	if ct < minSemanticCosine || cm < minSemanticCosine {
		t.Fatalf("related pairs under the noise floor: typo=%v morph=%v", ct, cm)
	}
	if co >= minSemanticCosine {
		t.Fatalf("unrelated pair above the noise floor: %v", co)
	}
	// Turkish morphology: an inflected query still lands on the base record.
	srv := Cosine(Embed("servis çalışıyor ve sağlıklı"), Embed("servisler çalışmıyor"))
	if srv < minSemanticCosine {
		t.Fatalf("turkish morphology cosine = %v", srv)
	}
	if Cosine(nil, base) != 0 || Cosine(base, nil) != 0 {
		t.Fatal("nil vectors must score 0")
	}
}

func semRecords() []Record {
	return []Record{
		{ID: "k8s", Subject: "kubernetes", Content: "the kubernetes cluster runs in frankfurt", Confidence: 1, LastSeenMS: 1000},
		{ID: "pizza", Subject: "cooking", Content: "pizza dough requires slow fermentation", Confidence: 1, LastSeenMS: 1000},
		{ID: "dead", Subject: "kubernetes", Content: "kubernetes node pool was resized", Confidence: 1, LastSeenMS: 1000, Tombstoned: true},
	}
}

// TestSearchSemantic_TypoFindsRecord is the headline capability: zero
// keyword overlap, the embedding still finds it.
func TestSearchSemantic_TypoFindsRecord(t *testing.T) {
	rs := semRecords()
	if hits := Search(rs, "kubenetes clstr", 5, 2000); len(hits) != 0 {
		t.Fatalf("keyword search unexpectedly matched: %v", hits)
	}
	hits := SearchSemantic(rs, "kubenetes clstr", 5, 2000)
	if len(hits) != 1 || hits[0].Record.ID != "k8s" {
		t.Fatalf("semantic hits = %+v", hits)
	}
	// Tombstoned and unrelated records stay out.
	for _, h := range hits {
		if h.Record.ID == "dead" || h.Record.ID == "pizza" {
			t.Fatalf("unwanted hit %s", h.Record.ID)
		}
	}
}

// TestSearchSemantic_ChattyQueryNotDiluted: a salient (misspelled) term
// buried in a conversational question still finds its record — per-token
// cosine rescues what whole-query dilution would drop below the floor.
func TestSearchSemantic_ChattyQueryNotDiluted(t *testing.T) {
	rs := semRecords()
	hits := SearchSemantic(rs, "what do you remember about kubenetes?", 5, 2000)
	if len(hits) != 1 || hits[0].Record.ID != "k8s" {
		t.Fatalf("chatty-query hits = %+v", hits)
	}
	// And the per-token path must not drag in unrelated records.
	if hits := SearchSemantic(rs, "what do you remember about quantum entanglement?", 5, 2000); len(hits) != 0 {
		t.Fatalf("unrelated chatty query matched: %+v", hits)
	}
}

// TestSearchHybrid_SupersetAndOrdering: every keyword hit survives hybrid,
// both-signal records outrank single-signal ones, and the typo case rides in.
func TestSearchHybrid_SupersetAndOrdering(t *testing.T) {
	rs := semRecords()

	// Exact query: the kubernetes record matches BOTH signals and must rank
	// above anything matching one.
	hy := SearchHybrid(rs, "kubernetes cluster", 5, 2000)
	if len(hy) == 0 || hy[0].Record.ID != "k8s" {
		t.Fatalf("hybrid top = %+v", hy)
	}
	kw := Search(rs, "kubernetes cluster", 5, 2000)
	if len(kw) == 0 {
		t.Fatal("keyword baseline empty")
	}
	if hy[0].Score <= kw[0].Score {
		t.Fatalf("both-signal score %v must exceed keyword-only %v", hy[0].Score, kw[0].Score)
	}
	// Every keyword hit appears in the hybrid result (superset property).
	hybridIDs := map[string]bool{}
	for _, h := range hy {
		hybridIDs[h.Record.ID] = true
	}
	for _, h := range kw {
		if !hybridIDs[h.Record.ID] {
			t.Fatalf("keyword hit %s lost in hybrid", h.Record.ID)
		}
	}

	// Typo query: keyword finds nothing, hybrid still surfaces the record.
	hy = SearchHybrid(rs, "kubenetes clstr", 5, 2000)
	if len(hy) != 1 || hy[0].Record.ID != "k8s" {
		t.Fatalf("hybrid typo hits = %+v", hy)
	}

	// Limit applies after the merge.
	if got := SearchHybrid(rs, "kubernetes pizza fermentation", 1, 2000); len(got) != 1 {
		t.Fatalf("limit ignored: %d hits", len(got))
	}
}

// TestSearchSemantic_Determinism: two identical calls produce identical
// ordering (the sort ties on LastSeenMS then ID).
func TestSearchSemantic_Determinism(t *testing.T) {
	rs := []Record{
		{ID: "a", Subject: "deploy", Content: "deploy service to prod", Confidence: 1, LastSeenMS: 100},
		{ID: "b", Subject: "deploy", Content: "deploy service to prod", Confidence: 1, LastSeenMS: 100},
	}
	x := SearchSemantic(rs, "deploying services", 5, 200)
	y := SearchSemantic(rs, "deploying services", 5, 200)
	if len(x) != 2 || len(y) != 2 || x[0].Record.ID != y[0].Record.ID || x[0].Record.ID != "a" {
		t.Fatalf("non-deterministic: %+v vs %+v", x, y)
	}
}
