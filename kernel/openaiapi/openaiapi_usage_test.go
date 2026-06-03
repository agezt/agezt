// SPDX-License-Identifier: MIT

package openaiapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// With stream_options.include_usage, the stream ends with a usage-only chunk
// (choices:[] + usage) before [DONE] — the OpenAI shape cost-tracking clients
// rely on (M237).
func TestChatStreaming_IncludeUsage(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hello world", tokens: []string{"hello", " world"}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi there"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"usage"`) || !strings.Contains(out, `"completion_tokens"`) {
		t.Errorf("include_usage should add a usage chunk:\n%s", out)
	}
	if !strings.Contains(out, `"choices":[]`) {
		t.Errorf("the usage chunk should carry empty choices:\n%s", out)
	}
	// The usage chunk must come before the [DONE] terminator.
	if u, d := strings.Index(out, `"usage"`), strings.LastIndex(out, "data: [DONE]"); u < 0 || d < 0 || u > d {
		t.Errorf("usage chunk must precede [DONE] (usage@%d done@%d)", u, d)
	}
}

// Without stream_options.include_usage, no usage chunk is emitted (matches
// OpenAI's default streaming behaviour).
func TestChatStreaming_NoUsageByDefault(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hi", tokens: []string{"hi"}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"usage"`) {
		t.Errorf("no usage chunk should appear without stream_options.include_usage:\n%s", rec.Body.String())
	}
}
