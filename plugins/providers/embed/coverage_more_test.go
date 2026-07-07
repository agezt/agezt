// SPDX-License-Identifier: MIT

package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedCoverageValidationAndEmptyBatch(t *testing.T) {
	if _, err := (&Client{BaseURL: "http://example"}).EmbedBatch(context.Background(), []string{"x"}); err == nil || !strings.Contains(err.Error(), "model required") {
		t.Fatalf("missing model error = %v", err)
	}
	if _, err := (&Client{Model: "m"}).EmbedBatch(context.Background(), []string{"x"}); err == nil || !strings.Contains(err.Error(), "base URL required") {
		t.Fatalf("missing base error = %v", err)
	}
	got, err := (&Client{BaseURL: "http://example", Model: "m"}).EmbedBatch(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("empty batch = %#v err %v, want nil nil", got, err)
	}
}

func TestEmbedCoverageHTTPAndDecodeErrors(t *testing.T) {
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
		{name: "bad index", fn: func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(embedResponse{Data: []struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{{Index: 0, Embedding: []float32{1, 0}}, {Index: 2, Embedding: []float32{1, 0}}}})
		}, want: "out of range"},
		{name: "missing index", fn: func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(embedResponse{Data: []struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{{Index: 0, Embedding: []float32{1, 0}}, {Index: 0, Embedding: []float32{1, 0}}}})
		}, want: "missing embedding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tc.fn(w) }))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, Model: "m", HTTP: srv.Client()}
			_, err := c.EmbedBatch(context.Background(), []string{"a", "b"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestEmbedCoverageNilHTTPAndZeroNormalize(t *testing.T) {
	if got := normalize([]float32{0, 0}); len(got) != 2 || got[0] != 0 || got[1] != 0 {
		t.Fatalf("zero normalize = %#v", got)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": []float64{2, 0}}}})
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Model: "m", HTTP: nil}
	// Point the default client at the httptest URL by using the public server URL;
	// nil HTTP exercises the client fallback branch without needing a custom transport.
	vecs, err := c.EmbedBatch(context.Background(), []string{"x"})
	if err != nil {
		t.Fatalf("EmbedBatch nil HTTP: %v", err)
	}
	if len(vecs) != 1 || vecs[0][0] != 1 || vecs[0][1] != 0 {
		t.Fatalf("normalized vecs = %#v", vecs)
	}
}
