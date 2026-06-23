// SPDX-License-Identifier: MIT

package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRerank(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// Return docs reordered: index 2 most relevant, then 0.
		resp := map[string]any{"results": []map[string]any{
			{"index": 2, "relevance_score": 0.9},
			{"index": 0, "relevance_score": 0.4},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "rerank-v3", "k")
	docs := []string{"alpha", "beta", "gamma"}
	idx, scores, err := c.Rerank(context.Background(), "best match", docs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/rerank" {
		t.Fatalf("path = %s", gotPath)
	}
	if len(idx) != 2 || idx[0] != 2 || idx[1] != 0 {
		t.Fatalf("ranking order wrong: %v", idx)
	}
	if scores[0] != 0.9 {
		t.Fatalf("score wrong: %v", scores)
	}
	if docs[idx[0]] != "gamma" {
		t.Fatalf("top doc should be gamma, got %s", docs[idx[0]])
	}
}

func TestRerank_EmptyDocs(t *testing.T) {
	c := New("http://x", "m", "")
	idx, _, err := c.Rerank(context.Background(), "q", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if idx != nil {
		t.Fatalf("empty docs should return nil, got %v", idx)
	}
}

func TestRerank_EndpointSuffixes(t *testing.T) {
	cases := map[string]string{
		"https://api.cohere.com/v2": "https://api.cohere.com/v2/rerank",
		"https://x/rerank":          "https://x/rerank",
		"https://x":                 "https://x/v1/rerank",
	}
	for in, want := range cases {
		c := &Client{BaseURL: in, Model: "m"}
		if got := c.endpoint(); got != want {
			t.Fatalf("endpoint(%q) = %q, want %q", in, got, want)
		}
	}
}
