// SPDX-License-Identifier: MIT

package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sampleHTML mimics the DuckDuckGo no-JS result markup the tool parses,
// including the //duckduckgo.com/l/?uddg= redirect wrapper.
const sampleHTML = `<!DOCTYPE html><html><body>
<div class="result">
  <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2Fagiresearch%2FAIOS">AIOS: <b>AI</b> Agent OS</a>
  <a class="result__snippet" href="x">AIOS is an <b>AI</b> agent operating &amp; system.</a>
</div>
<div class="result">
  <a rel="nofollow" class="result__a" href="https://example.com/agentic">Agentic OS direct link</a>
  <a class="result__snippet" href="y">A   plain    snippet with   spaces.</a>
</div>
</body></html>`

func newStub(t *testing.T, body string, status int) *Tool {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Tool{Endpoint: srv.URL, HTTP: srv.Client()}
}

func invoke(t *testing.T, tool *Tool, in map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	if uerr := json.Unmarshal([]byte(res.Output), &out); uerr != nil {
		t.Fatalf("output not JSON (%q): %v", res.Output, uerr)
	}
	return out
}

func TestDefinition(t *testing.T) {
	d := New().Definition()
	if d.Name != "web_search" {
		t.Fatalf("name = %q, want web_search", d.Name)
	}
	if !json.Valid(d.InputSchema) {
		t.Fatal("input schema is not valid JSON")
	}
}

func TestParsesResultsAndDecodesRedirect(t *testing.T) {
	tool := newStub(t, sampleHTML, 200)
	out := invoke(t, tool, map[string]any{"query": "agentic os"})

	if out["count"].(float64) != 2 {
		t.Fatalf("count = %v, want 2", out["count"])
	}
	results := out["results"].([]any)
	first := results[0].(map[string]any)
	if first["url"] != "https://github.com/agiresearch/AIOS" {
		t.Errorf("redirect not decoded: url = %q", first["url"])
	}
	if first["title"] != "AIOS: AI Agent OS" {
		t.Errorf("title tags/entities not stripped: %q", first["title"])
	}
	if first["snippet"] != "AIOS is an AI agent operating & system." {
		t.Errorf("snippet not cleaned: %q", first["snippet"])
	}
	// Whitespace in the second snippet should be collapsed.
	second := results[1].(map[string]any)
	if second["snippet"] != "A plain snippet with spaces." {
		t.Errorf("whitespace not collapsed: %q", second["snippet"])
	}
}

func TestLimitCapsResults(t *testing.T) {
	tool := newStub(t, sampleHTML, 200)
	out := invoke(t, tool, map[string]any{"query": "agentic os", "limit": 1})
	if out["count"].(float64) != 1 {
		t.Fatalf("count = %v, want 1 (limit honoured)", out["count"])
	}
}

func TestEmptyQueryIsError(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"query":"  "}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("empty query should be an error result")
	}
}

func TestFailSoftOnHTTPError(t *testing.T) {
	tool := newStub(t, "boom", 503)
	out := invoke(t, tool, map[string]any{"query": "x"})
	if out["count"].(float64) != 0 {
		t.Fatalf("count = %v, want 0 on HTTP error", out["count"])
	}
	note, _ := out["note"].(string)
	if !strings.Contains(note, "503") {
		t.Errorf("note should explain the HTTP error, got %q", note)
	}
}

func TestNoResultsIsSoft(t *testing.T) {
	tool := newStub(t, "<html><body>nothing here</body></html>", 200)
	out := invoke(t, tool, map[string]any{"query": "x"})
	if out["count"].(float64) != 0 {
		t.Fatalf("count = %v, want 0", out["count"])
	}
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"query":"x"}`))
	if res.IsError {
		t.Error("a no-result search must not be an error result")
	}
}
