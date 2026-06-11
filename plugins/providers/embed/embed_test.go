// SPDX-License-Identifier: MIT

package embed_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/plugins/providers/embed"
)

// TestEmbedBatch_RequestShapeAndNormalization (M901): one POST carries the
// whole batch; vectors come back in INPUT order even when the wire delivers
// them out of order, and every vector is L2-normalized (the kernel's cosine
// is a bare dot product).
func TestEmbedBatch_RequestShapeAndNormalization(t *testing.T) {
	var seen struct {
		path, auth string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		// Deliberately out of order + un-normalized.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 1, "embedding": []float64{0, 5, 0}},
				{"index": 0, "embedding": []float64{3, 4, 0}},
			},
		})
	}))
	defer srv.Close()

	c := embed.New(srv.URL+"/v1", "test-model", "sk-em")
	vecs, err := c.EmbedBatch(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if seen.path != "/v1/embeddings" {
		t.Errorf("path = %q, want /v1/embeddings", seen.path)
	}
	if seen.auth != "Bearer sk-em" {
		t.Errorf("auth = %q", seen.auth)
	}
	if seen.body["model"] != "test-model" {
		t.Errorf("model = %v", seen.body["model"])
	}
	if in, _ := seen.body["input"].([]any); len(in) != 2 || in[0] != "first" {
		t.Errorf("input = %v, want the batch in order", seen.body["input"])
	}

	if len(vecs) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vecs))
	}
	// Index 0 was delivered second on the wire: [3,4,0] → normalized [0.6,0.8,0].
	if math.Abs(float64(vecs[0][0])-0.6) > 1e-6 || math.Abs(float64(vecs[0][1])-0.8) > 1e-6 {
		t.Errorf("vecs[0] = %v, want normalized [0.6 0.8 0] (wire order honoured by index)", vecs[0])
	}
	for i, v := range vecs {
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		if math.Abs(sum-1) > 1e-5 {
			t.Errorf("vecs[%d] L2² = %f, want 1 (normalized)", i, sum)
		}
	}
}

// TestEmbedBatch_BareHostGetsV1 (M901): an Ollama-style bare host URL gains
// the /v1/embeddings path automatically.
func TestEmbedBatch_BareHostGetsV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.Error(w, "wrong path "+r.URL.Path, http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected auth", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float64{1, 0}}},
		})
	}))
	defer srv.Close()

	c := embed.New(srv.URL, "nomic-embed-text", "") // bare host, no key — the local Ollama shape
	vecs, err := c.EmbedBatch(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("vectors = %d, want 1", len(vecs))
	}
}

// TestEmbedBatch_CountMismatchIsError (M901): a server returning fewer
// embeddings than inputs must error — silently misaligned vectors would
// corrupt the kernel's per-record cache.
func TestEmbedBatch_CountMismatchIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float64{1, 0}}},
		})
	}))
	defer srv.Close()

	c := embed.New(srv.URL, "m", "")
	if _, err := c.EmbedBatch(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("EmbedBatch succeeded with 1 vector for 2 inputs, want an error")
	}
}
