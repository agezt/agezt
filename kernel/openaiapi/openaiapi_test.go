// SPDX-License-Identifier: MIT

package openaiapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// fakeEngine implements Engine. RunWith publishes its tokens on the bus under
// the run subject (exercising the real SSE path) then returns its answer.
type fakeEngine struct {
	b         *bus.Bus
	answer    string
	tokens    []string
	model     string
	models    []string
	ranIntent string
	ranModel  string
}

func (f *fakeEngine) NewCorrelation() string        { return "test-corr" }
func (f *fakeEngine) SubjectForRun(c string) string { return "agent.agent-" + c + ".llm" }
func (f *fakeEngine) DefaultModel() string          { return f.model }
func (f *fakeEngine) ModelIDs() []string            { return f.models }
func (f *fakeEngine) RunModel(_ context.Context, corr, intent, model string) (string, error) {
	f.ranIntent = intent
	f.ranModel = model
	for _, tok := range f.tokens {
		_, _ = f.b.PublishStreaming(event.Spec{
			Subject:       f.SubjectForRun(corr),
			Kind:          event.KindLLMToken,
			Actor:         "agent-" + corr,
			CorrelationID: corr,
			Payload:       map[string]any{"text": tok},
		})
	}
	return f.answer, nil
}

func newAPIServer(t *testing.T, eng *fakeEngine, token string) *Server {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	eng.b = b
	return New(eng, b, token)
}

func TestModelsListsDefaultAndCatalog(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o", "MiniMax-M2.7"}}
	s := newAPIServer(t, eng, "secret")
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var out struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" {
		t.Errorf("object=%q", out.Object)
	}
	// Default model present, no duplicate despite also being in the catalog.
	ids := map[string]int{}
	for _, m := range out.Data {
		ids[m["id"].(string)]++
	}
	if ids["MiniMax-M2.7"] != 1 || ids["gpt-4o"] != 1 {
		t.Errorf("model ids = %v (want each once)", ids)
	}
}

func TestAuthRequired(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "secret")
	for _, tc := range []struct {
		name string
		set  func(*http.Request)
	}{
		{"no token", func(*http.Request) {}},
		{"wrong bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }},
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		tc.set(req)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status=%d want 401", tc.name, rec.Code)
		}
	}
}

func TestEmptyTokenNeverAuthorizes(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "") // no token configured
	req := httptest.NewRequest(http.MethodGet, "/v1/models?token=", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("empty token must fail closed, got %d", rec.Code)
	}
}

func TestChatCompletionNonStreaming(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "module github.com/agezt/agezt"}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"what is this project?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Corr string `json:"agezt_correlation_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "chat.completion" || out.Model != "gpt-4o" {
		t.Errorf("object=%q model=%q", out.Object, out.Model)
	}
	if len(out.Choices) != 1 || out.Choices[0].Message.Content != "module github.com/agezt/agezt" {
		t.Errorf("choices = %+v", out.Choices)
	}
	if out.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason=%q", out.Choices[0].FinishReason)
	}
	if out.Corr == "" {
		t.Error("expected agezt_correlation_id for `agt why`")
	}
	// The user content reached the engine verbatim (single-turn → clean intent).
	if eng.ranIntent != "what is this project?" {
		t.Errorf("intent = %q", eng.ranIntent)
	}
	// The requested model is honoured (per-request routing): the engine ran
	// under "gpt-4o", not the configured default "m".
	if eng.ranModel != "gpt-4o" {
		t.Errorf("ranModel = %q, want the requested gpt-4o", eng.ranModel)
	}
}

func TestChatRequestedModelHonoured_Default(t *testing.T) {
	// No model in the request → the engine runs under the configured default.
	eng := &fakeEngine{model: "MiniMax-M2.7", answer: "ok"}
	s := newAPIServer(t, eng, "secret")
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if eng.ranModel != "MiniMax-M2.7" {
		t.Errorf("ranModel = %q, want default MiniMax-M2.7", eng.ranModel)
	}
}

func TestChatCompletionStreaming(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hello world", tokens: []string{"hello", " world"}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type=%q", ct)
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"role":"assistant"`) {
		t.Error("missing opening role chunk")
	}
	if !strings.Contains(out, `"content":"hello"`) || !strings.Contains(out, `"content":" world"`) {
		t.Errorf("missing token deltas in:\n%s", out)
	}
	if !strings.Contains(out, `"finish_reason":"stop"`) {
		t.Error("missing stop chunk")
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Error("missing [DONE] terminator")
	}
}

func TestChatRejectsGET(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "secret")
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on chat should be 405, got %d", rec.Code)
	}
}

func TestChatEmptyMessages(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "secret")
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty messages should be 400, got %d", rec.Code)
	}
}

func TestIntentFromMessages(t *testing.T) {
	// Single user turn → verbatim.
	got := intentFromMessages([]chatMessage{{Role: "user", Content: json.RawMessage(`"summarise the repo"`)}})
	if got != "summarise the repo" {
		t.Errorf("single-turn intent = %q", got)
	}

	// System + multi-turn → guidance prefix + labelled transcript.
	got = intentFromMessages([]chatMessage{
		{Role: "system", Content: json.RawMessage(`"be terse"`)},
		{Role: "user", Content: json.RawMessage(`"hi"`)},
		{Role: "assistant", Content: json.RawMessage(`"hello"`)},
		{Role: "user", Content: json.RawMessage(`"now what"`)},
	})
	if !strings.HasPrefix(got, "be terse") {
		t.Errorf("system guidance missing: %q", got)
	}
	if !strings.Contains(got, "User: hi") || !strings.Contains(got, "Assistant: hello") || !strings.Contains(got, "User: now what") {
		t.Errorf("transcript labels missing: %q", got)
	}

	// Array content parts are flattened to text.
	got = intentFromMessages([]chatMessage{
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"part one"},{"type":"text","text":"part two"}]`)},
	})
	if got != "part one\npart two" {
		t.Errorf("array content flatten = %q", got)
	}
}
