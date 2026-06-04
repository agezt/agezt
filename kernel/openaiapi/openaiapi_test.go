// SPDX-License-Identifier: MIT

package openaiapi

import (
	"context"
	"encoding/json"
	"errors"
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
	b           *bus.Bus
	answer      string
	tokens      []string
	reasoning   []string // llm.reasoning deltas to emit before the answer (M323)
	model       string
	models      []string
	ranIntent   string
	ranModel    string
	ranImages   []string
	ranJSONMode bool
}

func (f *fakeEngine) NewCorrelation() string        { return "test-corr" }
func (f *fakeEngine) SubjectForRun(c string) string { return "agent.agent-" + c + ".llm" }
func (f *fakeEngine) DefaultModel() string          { return f.model }
func (f *fakeEngine) ModelIDs() []string            { return f.models }
func (f *fakeEngine) RunModel(_ context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error) {
	f.ranIntent = intent
	f.ranModel = model
	f.ranImages = images
	f.ranJSONMode = jsonMode
	for _, rt := range f.reasoning {
		_, _ = f.b.PublishStreaming(event.Spec{
			Subject:       f.SubjectForRun(corr),
			Kind:          event.KindLLMReasoning,
			Actor:         "agent-" + corr,
			CorrelationID: corr,
			Payload:       map[string]any{"text": rt},
		})
	}
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

func TestChat_TenantRouting(t *testing.T) {
	// Primary engine + a separate tenant engine with its own bus.
	primary := &fakeEngine{answer: "primary-answer", model: "m"}
	s := newAPIServer(t, primary, "secret")

	tj, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tbus := bus.New(tj)
	t.Cleanup(func() { tbus.Close(); tj.Close() })
	alpha := &fakeEngine{answer: "alpha-answer", model: "m", b: tbus}

	s.SetTenantResolver(func(id string) (Engine, *bus.Bus, error) {
		if id == "alpha" {
			return alpha, tbus, nil
		}
		return nil, nil, errors.New("unknown tenant " + id)
	})

	chat := func(tenant, intent string) *httptest.ResponseRecorder {
		body := `{"model":"m","messages":[{"role":"user","content":"` + intent + `"}]}`
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer secret")
		if tenant != "" {
			r.Header.Set("X-Agezt-Tenant", tenant)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec
	}

	// No header → primary engine.
	if rec := chat("", "hello"); rec.Code != http.StatusOK || primary.ranIntent != "hello" {
		t.Fatalf("primary route: code=%d ran=%q", rec.Code, primary.ranIntent)
	}
	if alpha.ranIntent != "" {
		t.Error("tenant engine should not have run for a header-less request")
	}

	// X-Agezt-Tenant: alpha → tenant engine, isolated.
	rec := chat("alpha", "for-alpha")
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant route status=%d", rec.Code)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Choices) == 0 || out.Choices[0].Message.Content != "alpha-answer" {
		t.Errorf("answer = %+v, want alpha-answer", out.Choices)
	}
	if alpha.ranIntent != "for-alpha" {
		t.Errorf("tenant engine ran %q, want for-alpha", alpha.ranIntent)
	}
	if primary.ranIntent != "hello" {
		t.Error("primary engine must not have run the tenant request")
	}

	// Unknown tenant → 400 from the resolver error.
	if rec := chat("ghost", "x"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown tenant status = %d, want 400", rec.Code)
	}
}

// A per-tenant token authorizes ONLY its own tenant on the OpenAI surface; the
// admin token authorizes anything; a tenant token needs the matching header.
func TestTenantAuth(t *testing.T) {
	eng := &fakeEngine{model: "m", models: []string{"m"}}
	s := newAPIServer(t, eng, "admin-tok")
	s.SetTenantAuthorizer(func(id, presented string) bool {
		return id == "alpha" && presented == "alpha-tok"
	})

	req := func(token, tenant string) int {
		r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if tenant != "" {
			r.Header.Set("X-Agezt-Tenant", tenant)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec.Code
	}

	cases := []struct {
		name          string
		token, tenant string
		want          int
	}{
		{"admin no tenant", "admin-tok", "", http.StatusOK},
		{"admin any tenant", "admin-tok", "alpha", http.StatusOK},
		{"tenant token own tenant", "alpha-tok", "alpha", http.StatusOK},
		{"tenant token wrong tenant", "alpha-tok", "beta", http.StatusUnauthorized},
		{"tenant token no header", "alpha-tok", "", http.StatusUnauthorized},
		{"no token", "", "alpha", http.StatusUnauthorized},
	}
	for _, c := range cases {
		if got := req(c.token, c.tenant); got != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, got, c.want)
		}
	}
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

// A reasoning model's chain of thought (llm.reasoning events) must surface on the
// non-streaming response as message.reasoning_content — the DeepSeek-R1 convention
// — without leaking into the answer content (M323).
func TestChatCompletionNonStreaming_ReasoningContent(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "42", reasoning: []string{"6*7 ", "= 42."}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","messages":[{"role":"user","content":"6*7?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Choices) != 1 {
		t.Fatalf("choices=%+v", out.Choices)
	}
	if out.Choices[0].Message.ReasoningContent != "6*7 = 42." {
		t.Errorf("reasoning_content=%q", out.Choices[0].Message.ReasoningContent)
	}
	if out.Choices[0].Message.Content != "42" {
		t.Errorf("content=%q (reasoning must not leak into the answer)", out.Choices[0].Message.Content)
	}
}

// A non-reasoning run must omit reasoning_content entirely (not an empty string a
// client would have to special-case) — the response stays byte-identical to before.
func TestChatCompletionNonStreaming_NoReasoningKeyWhenAbsent(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hi"}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "reasoning_content") {
		t.Errorf("reasoning_content must be absent for non-reasoning runs:\n%s", rec.Body.String())
	}
}

// In streaming mode, reasoning deltas must arrive as `reasoning_content` deltas,
// distinct from the answer's `content` deltas (M323).
func TestChatCompletionStreaming_ReasoningContent(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "42", tokens: []string{"42"}, reasoning: []string{"6*7 ", "= 42."}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","stream":true,"messages":[{"role":"user","content":"6*7?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"reasoning_content":"6*7 "`) || !strings.Contains(out, `"reasoning_content":"= 42."`) {
		t.Errorf("missing reasoning_content deltas in:\n%s", out)
	}
	if !strings.Contains(out, `"content":"42"`) {
		t.Errorf("missing answer content delta in:\n%s", out)
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

// TestChat_ResponseFormatJSONMode (M314): a client's response_format:{json_object}
// flows to the run as JSON mode; absence leaves it off.
func TestChat_ResponseFormatJSONMode(t *testing.T) {
	post := func(body string) *fakeEngine {
		eng := &fakeEngine{answer: "{}", model: "m"}
		s := newAPIServer(t, eng, "secret")
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
		return eng
	}

	on := post(`{"model":"m","messages":[{"role":"user","content":"give me json"}],"response_format":{"type":"json_object"}}`)
	if !on.ranJSONMode {
		t.Error("response_format json_object should set JSON mode on the run")
	}
	off := post(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	if off.ranJSONMode {
		t.Error("no response_format must leave JSON mode off")
	}
	// json_schema also counts as structured.
	sch := post(`{"model":"m","messages":[{"role":"user","content":"x"}],"response_format":{"type":"json_schema"}}`)
	if !sch.ranJSONMode {
		t.Error("response_format json_schema should set JSON mode")
	}
}
