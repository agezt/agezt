// SPDX-License-Identifier: MIT

package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestWebsearchCoverageDefinitionAndHelpers2(t *testing.T) {
	// client() returns injected HTTP when set, otherwise builds a netguard
	// client with the requested options.
	tl := New()
	custom := &http.Client{}
	tl.HTTP = custom
	if tl.client() != custom {
		t.Fatal("client() should return the injected client")
	}
	tl.HTTP = nil
	if tl.client() == nil {
		t.Fatal("default client() should be non-nil")
	}
	if tl.client() == custom {
		t.Fatal("default client() should not be the injected client")
	}

	def := tl.Definition()
	if def.Name != "web_search" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	if def.Effect.Confidence <= 0 {
		t.Fatalf("Confidence should be > 0, got %v", def.Effect.Confidence)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"query"`, `"limit"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should include %q, got %s", want, schema)
		}
	}
}

func TestWebsearchCoverageInvokeValidation2(t *testing.T) {
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	tl := New()
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"query":""}`))
	if !res.IsError || !strings.Contains(res.Output, "query required") {
		t.Fatalf("empty query = %+v", res)
	}
}

func TestWebsearchCoverageParseAndHelpers2(t *testing.T) {
	if got := parseResults("", 5); len(got) != 0 {
		t.Fatalf("empty body = %v, want empty", got)
	}

	// The snippet regex looks for <td class='result-snippet'> just after each
	// <a class='result-link'> link. The lite markup typically places the snippet
	// cell on a separate line. The link regex expects `href` BEFORE `class`
	// (matching the actual DuckDuckGo lite order).
	body := `<a href="https://a.example" class="result-link">A Title</a>
<td class="result-snippet">A snippet</td>
<a href="https://b.example" class="result-link">B Title</a>
<td class="result-snippet">B snippet</td>`
	got := parseResults(body, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(got), got)
	}
	if got[0].URL != "https://a.example" {
		t.Fatalf("URL = %q", got[0].URL)
	}
	if got[0].Title != "A Title" {
		t.Fatalf("Title = %q", got[0].Title)
	}
	if got[0].Snippet != "A snippet" {
		t.Fatalf("Snippet = %q", got[0].Snippet)
	}

	// limit=1 clamps to one result.
	got2 := parseResults(body, 1)
	if len(got2) != 1 {
		t.Fatalf("limit=1 got %d", len(got2))
	}

	// Empty title or empty URL is skipped.
	body3 := `<a href="" class="result-link"></a>
<a href="https://a.example" class="result-link"></a>
<a href="https://b.example" class="result-link">Real</a>`
	got3 := parseResults(body3, 5)
	if len(got3) != 1 || got3[0].Title != "Real" {
		t.Fatalf("expected only Real link: %+v", got3)
	}

	// cleanURL branches.
	if got := cleanURL("//example.com/x"); got != "https://example.com/x" {
		t.Fatalf("scheme-less // got %q", got)
	}
	if got := cleanURL("https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Freal&foo=bar"); got != "https://example.com/real" {
		t.Fatalf("uddg not unwrapped: %q", got)
	}
	if got := cleanURL(""); got != "" {
		t.Fatalf("empty URL = %q", got)
	}
	if got := cleanURL("ftp://example.com"); got != "" {
		t.Fatalf("non-http URL should be rejected: %q", got)
	}

	// cleanText.
	if got := cleanText("  <b>Tom &amp; Jerry</b>  "); got != "Tom & Jerry" {
		t.Fatalf("cleanText = %q", got)
	}
}

func TestWebsearchCoverageHTTPHappyPath2(t *testing.T) {
	body := `<a href="https://a.example" class="result-link">A Title</a>
<td class="result-snippet">A snippet</td>
<a href="https://b.example" class="result-link">B Title</a>
<td class="result-snippet">B snippet</td>`

	var gotPath, gotQuery, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("q")
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tl := New()
	tl.Endpoint = srv.URL
	tl.HTTP = srv.Client()
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"query":"foo","limit":2}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	if gotPath != "/lite/" && gotPath != "/" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "foo" {
		t.Fatalf("query = %q", gotQuery)
	}
	if !strings.Contains(gotUA, "Mozilla") {
		t.Fatalf("UA = %q", gotUA)
	}
	for _, want := range []string{`"count": 2`, `"A Title"`, `"https://a.example"`, `"A snippet"`} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q: %s", want, res.Output)
		}
	}
}

func TestWebsearchCoverageHTTPErrors2(t *testing.T) {
	// 4xx response → soft error (no IsError).
	srv4xx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv4xx.Close()
	tl := New()
	tl.Endpoint = srv4xx.URL
	tl.HTTP = srv4xx.Client()
	res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"query":"foo"}`))
	if res.IsError {
		t.Fatalf("4xx should be soft: %+v", res)
	}
	if !strings.Contains(res.Output, "HTTP 403") {
		t.Fatalf("4xx note missing: %s", res.Output)
	}

	// No matches → soft empty result.
	srvNoMatch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>no links</body></html>`))
	}))
	defer srvNoMatch.Close()
	tl2 := New()
	tl2.Endpoint = srvNoMatch.URL
	tl2.HTTP = srvNoMatch.Client()
	res, _ = tl2.Invoke(context.Background(), json.RawMessage(`{"query":"foo"}`))
	if !strings.Contains(res.Output, `"count": 0`) {
		t.Fatalf("no-match should report count=0: %s", res.Output)
	}

	// Custom UA honored.
	srvUA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "TestAgent/1.0" {
			t.Errorf("UA = %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte(`<a class="result-link" href="https://x">X</a>`))
	}))
	defer srvUA.Close()
	tl3 := New()
	tl3.Endpoint = srvUA.URL
	tl3.HTTP = srvUA.Client()
	tl3.UserAgent = "TestAgent/1.0"
	_, _ = tl3.Invoke(context.Background(), json.RawMessage(`{"query":"q","limit":1000}`))
}
