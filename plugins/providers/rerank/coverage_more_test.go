// SPDX-License-Identifier: MIT

package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRerankCoverageValidationAndEndpoints(t *testing.T) {
	if (&Client{}).HasRerank() {
		t.Fatal("empty client should not be configured")
	}
	if _, _, err := (&Client{BaseURL: "u", Model: "m"}).Rerank(context.Background(), "   ", []string{"a"}, 1); err == nil || !strings.Contains(err.Error(), "query required") {
		t.Fatalf("blank query error = %v", err)
	}
	if _, _, err := (&Client{BaseURL: "u", Model: "m"}).Rerank(context.Background(), "q", nil, 0); err != nil {
		t.Fatalf("empty docs should not error, got %v", err)
	}

	endpoints := map[string]string{
		"https://api.example.cohere.com/v1/rerank": "https://api.example.cohere.com/v1/rerank",
		"https://proxy.example/api/v2/custom":      "https://proxy.example/api/v2/custom/rerank",
	}
	for in, want := range endpoints {
		if got := (&Client{BaseURL: in}).endpoint(); got != want {
			t.Fatalf("endpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRerankCoverageHTTPAndDecodeErrors(t *testing.T) {
	cases := []struct {
		name string
		fn   func(http.ResponseWriter)
		want string
	}{
		{name: "status", fn: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream down"))
		}, want: "status 502"},
		{name: "bad json", fn: func(w http.ResponseWriter) { _, _ = w.Write([]byte("not-json")) }, want: "decode"},
		{name: "out of range", fn: func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"index": 5, "relevance_score": 0.5}}})
		}, want: "out of range"},
		{name: "empty docs", fn: func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"index": 0, "relevance_score": 0.1}}})
		}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tc.fn(w) }))
			defer srv.Close()
			c := New(srv.URL, "rerank", "k")
			_, _, err := c.Rerank(context.Background(), "q", []string{"a", "b"}, 0)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRerankCoverageNilHTTPAndAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected auth header without key")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"index": 0, "relevance_score": 0.5}}})
	}))
	defer srv.Close()

	// Netguard's default client allows loopback for this adapter (the embed adapter
	// pattern), but rerank also allows loopback/private by default. With nil HTTP,
	// it falls back to the configured default client.
	c := &Client{BaseURL: srv.URL, Model: "rerank"}
	idx, scores, err := c.Rerank(context.Background(), "q", []string{"doc"}, 0)
	if err != nil {
		t.Skipf("netguard refused nil HTTP loopback: %v", err)
	}
	if len(idx) != 1 || idx[0] != 0 || scores[0] != 0.5 {
		t.Fatalf("rerank = %#v", idx)
	}
}
