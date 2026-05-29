// SPDX-License-Identifier: MIT

package browser_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/plugins/tools/browser"
)

// ---- HTMLToText unit tests ----

func TestHTMLToText_StripsScriptStyleNoscript(t *testing.T) {
	in := `<html><head>
<script>alert("hi")</script>
<style>body { color: red; }</style>
<noscript>JS disabled</noscript>
</head><body>visible</body></html>`
	got := browser.HTMLToText(in)
	if strings.Contains(got, "alert") {
		t.Errorf("script content leaked: %q", got)
	}
	if strings.Contains(got, "color: red") {
		t.Errorf("style content leaked: %q", got)
	}
	if strings.Contains(got, "JS disabled") {
		t.Errorf("noscript content leaked: %q", got)
	}
	if !strings.Contains(got, "visible") {
		t.Errorf("visible body content lost: %q", got)
	}
}

func TestHTMLToText_DecodesEntities(t *testing.T) {
	in := `<p>Tom &amp; Jerry &lt;3 &#x1F600; &copy; 2026</p>`
	got := browser.HTMLToText(in)
	want := `Tom & Jerry <3 😀 © 2026`
	if got != want {
		t.Errorf("HTMLToText = %q, want %q", got, want)
	}
}

func TestHTMLToText_PreservesParagraphStructure(t *testing.T) {
	in := `<p>First para.</p><p>Second para.</p><h2>Heading</h2><p>Third.</p>`
	got := browser.HTMLToText(in)
	// Each block-level tag should produce at least one newline.
	// The exact whitespace is implementation detail, but distinct
	// paragraphs should be on separate lines.
	lines := strings.Split(got, "\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	if nonEmpty < 4 {
		t.Errorf("paragraph structure lost; got %d non-empty lines: %q", nonEmpty, got)
	}
}

func TestHTMLToText_StripsComments(t *testing.T) {
	in := `<!-- TODO: remove this --><p>Real content</p><!-- another -->`
	got := browser.HTMLToText(in)
	if strings.Contains(got, "TODO") || strings.Contains(got, "another") {
		t.Errorf("comments leaked: %q", got)
	}
	if !strings.Contains(got, "Real content") {
		t.Errorf("real content lost: %q", got)
	}
}

func TestHTMLToText_HandlesUnclosedScript(t *testing.T) {
	// Truncated download — script block opened but never closed.
	// We MUST NOT leak the script source into text output.
	in := `<p>Before</p><script>var leaked = "do not surface";`
	got := browser.HTMLToText(in)
	if strings.Contains(got, "leaked") {
		t.Errorf("unclosed-script content leaked: %q", got)
	}
	if !strings.Contains(got, "Before") {
		t.Errorf("pre-script content lost: %q", got)
	}
}

func TestHTMLToText_CollapsesWhitespace(t *testing.T) {
	in := `<p>much    space</p>


<p>and many newlines</p>`
	got := browser.HTMLToText(in)
	if strings.Contains(got, "    ") {
		t.Errorf("horizontal whitespace not collapsed: %q", got)
	}
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("vertical whitespace not collapsed: %q", got)
	}
}

func TestHTMLToText_HandlesEmptyAndPlainText(t *testing.T) {
	if got := browser.HTMLToText(""); got != "" {
		t.Errorf("empty input → %q, want empty", got)
	}
	// Plain text with no tags passes through (with whitespace normalised).
	if got := browser.HTMLToText("just text"); got != "just text" {
		t.Errorf("plain text → %q, want unchanged", got)
	}
}

func TestHTMLToText_StripsDoctype(t *testing.T) {
	in := `<!DOCTYPE html><html><body>content</body></html>`
	got := browser.HTMLToText(in)
	if strings.Contains(strings.ToLower(got), "doctype") {
		t.Errorf("doctype leaked: %q", got)
	}
	if got != "content" {
		t.Errorf("got %q, want 'content'", got)
	}
}

// ---- Invoke (network path) tests ----

func TestInvoke_FetchesAndExtracts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent header")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!DOCTYPE html>
<html><body>
<h1>Hello</h1>
<p>This is a test page.</p>
<script>analytics()</script>
</body></html>`)
	}))
	defer srv.Close()

	tool := browser.New()
	tool.AllowAll = true
	tool.HTTP = srv.Client()

	input := mustJSON(t, map[string]any{"url": srv.URL})
	res, err := tool.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	text, _ := out["text"].(string)
	if !strings.Contains(text, "Hello") {
		t.Errorf("text missing 'Hello': %q", text)
	}
	if !strings.Contains(text, "This is a test page.") {
		t.Errorf("text missing paragraph: %q", text)
	}
	if strings.Contains(text, "analytics") {
		t.Errorf("script leaked into text: %q", text)
	}
	if status, _ := out["status"].(float64); int(status) != 200 {
		t.Errorf("status = %v, want 200", status)
	}
}

func TestInvoke_DefaultDenyForUnallowedHost(t *testing.T) {
	tool := browser.New() // AllowAll=false, AllowedHosts empty
	input := mustJSON(t, map[string]any{"url": "https://example.com/"})
	_, err := tool.Invoke(context.Background(), input)
	if err == nil {
		t.Fatal("expected ErrHostDenied")
	}
	if !strings.Contains(err.Error(), "host not in allowlist") {
		t.Errorf("err = %v, want host-denied", err)
	}
}

func TestInvoke_RejectsNonHTTPScheme(t *testing.T) {
	tool := browser.New()
	tool.AllowAll = true
	input := mustJSON(t, map[string]any{"url": "file:///etc/passwd"})
	_, err := tool.Invoke(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Errorf("err = %v, want scheme-not-allowed", err)
	}
}

func TestInvoke_RejectsMissingURL(t *testing.T) {
	tool := browser.New()
	tool.AllowAll = true
	input := mustJSON(t, map[string]any{})
	_, err := tool.Invoke(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "url required") {
		t.Errorf("err = %v, want url-required", err)
	}
}

func TestInvoke_Surfaces4xxAsToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<h1>Not Found</h1>")
	}))
	defer srv.Close()

	tool := browser.New()
	tool.AllowAll = true
	tool.HTTP = srv.Client()
	_, err := tool.Invoke(context.Background(), mustJSON(t, map[string]any{"url": srv.URL}))
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("err = %v, want HTTP-404 surface", err)
	}
}

func TestInvoke_TruncatesLongContent(t *testing.T) {
	// Page with lots of text — verify max_chars truncation +
	// truncated_text flag.
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := 0; i < 500; i++ {
		sb.WriteString("<p>This is paragraph number ")
		sb.WriteString("[N]")
		sb.WriteString(" with some filler text to make it long.</p>")
	}
	sb.WriteString("</body></html>")
	page := sb.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, page)
	}))
	defer srv.Close()

	tool := browser.New()
	tool.AllowAll = true
	tool.HTTP = srv.Client()
	tool.MaxChars = 500 // tiny cap to force truncation
	res, err := tool.Invoke(context.Background(), mustJSON(t, map[string]any{"url": srv.URL}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	if v, _ := out["truncated_text"].(bool); !v {
		t.Errorf("truncated_text not set; output: %v", out)
	}
	text, _ := out["text"].(string)
	if !strings.Contains(text, "[truncated]") {
		t.Errorf("truncation marker missing in text: %q", text)
	}
}

func TestInvoke_PerCallMaxCharsCapsBelowToolDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html><body><p>"+strings.Repeat("x", 5000)+"</p></body></html>")
	}))
	defer srv.Close()

	tool := browser.New()
	tool.AllowAll = true
	tool.HTTP = srv.Client()
	// Tool default is 64KB; per-call cap of 100 chars.
	res, err := tool.Invoke(context.Background(), mustJSON(t, map[string]any{
		"url":       srv.URL,
		"max_chars": 100,
	}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	text, _ := out["text"].(string)
	// 100 chars + the "…[truncated]" marker.
	if len(text) > 200 {
		t.Errorf("per-call max_chars not honoured; len=%d", len(text))
	}
}

func TestDefinition_NameAndSchema(t *testing.T) {
	def := browser.New().Definition()
	if def.Name != "browser.read" {
		t.Errorf("Name = %q, want browser.read", def.Name)
	}
	// Schema should at least mention "url" as required.
	if !strings.Contains(string(def.InputSchema), `"url"`) {
		t.Errorf("schema missing url: %s", string(def.InputSchema))
	}
	if !strings.Contains(string(def.InputSchema), `"required"`) {
		t.Errorf("schema missing required: %s", string(def.InputSchema))
	}
}

func TestInvoke_WildcardHostMatch(t *testing.T) {
	tool := browser.New()
	tool.AllowedHosts = []string{"*.example.com"}
	// Direct match should fail (wildcard is one-level only).
	_, err := tool.Invoke(context.Background(), mustJSON(t, map[string]any{"url": "https://example.com/"}))
	if err == nil {
		t.Error("expected host-denied for bare example.com (wildcard requires subdomain)")
	}
	// Subdomain match should pass the host check (will fail at network
	// time but that proves the allowlist let it through).
	_, err = tool.Invoke(context.Background(), mustJSON(t, map[string]any{"url": "https://api.example.com/"}))
	if err != nil && strings.Contains(err.Error(), "host not in allowlist") {
		t.Errorf("wildcard didn't match api.example.com: %v", err)
	}
}

// ---- helper ----

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
