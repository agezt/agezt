// SPDX-License-Identifier: MIT

package openaiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponses_NonStreaming(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "the whole answer"}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"gpt-4o","input":"what is this?"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		OutputText string `json:"output_text"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Corr string `json:"agezt_correlation_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "response" || out.Model != "gpt-4o" || out.Status != "completed" {
		t.Errorf("object=%q model=%q status=%q", out.Object, out.Model, out.Status)
	}
	if out.OutputText != "the whole answer" {
		t.Errorf("output_text=%q", out.OutputText)
	}
	if len(out.Output) != 1 || out.Output[0].Role != "assistant" || out.Output[0].Type != "message" {
		t.Fatalf("output = %+v", out.Output)
	}
	c := out.Output[0].Content
	if len(c) != 1 || c[0].Type != "output_text" || c[0].Text != "the whole answer" {
		t.Errorf("content = %+v", c)
	}
	if out.Usage.TotalTokens != out.Usage.InputTokens+out.Usage.OutputTokens || out.Usage.OutputTokens == 0 {
		t.Errorf("usage = %+v", out.Usage)
	}
	if out.Corr == "" {
		t.Error("expected agezt_correlation_id")
	}
	// Per-request model + verbatim intent reached the engine.
	if eng.ranModel != "gpt-4o" {
		t.Errorf("ranModel=%q want gpt-4o", eng.ranModel)
	}
	if eng.ranIntent != "what is this?" {
		t.Errorf("intent=%q", eng.ranIntent)
	}
}

func TestResponses_InstructionsAndArrayInput(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "ok"}
	s := newAPIServer(t, eng, "secret")
	// instructions → system guidance; array input with typed parts.
	body := `{
		"instructions":"be terse",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"hello there"}]}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)

	if !strings.HasPrefix(eng.ranIntent, "be terse") {
		t.Errorf("instructions not surfaced as guidance: %q", eng.ranIntent)
	}
	if !strings.Contains(eng.ranIntent, "hello there") {
		t.Errorf("array input text not flattened: %q", eng.ranIntent)
	}
}

func TestResponses_Streaming(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hello world", tokens: []string{"hello", " world"}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","stream":true,"input":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
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
	for _, want := range []string{
		"event: response.created",
		"event: response.output_text.delta",
		`"delta":"hello"`,
		`"delta":" world"`,
		"event: response.output_text.done",
		`"text":"hello world"`,
		"event: response.completed",
		"data: [DONE]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stream missing %q in:\n%s", want, out)
		}
	}
}

// A reasoning model's chain of thought must surface on the non-streaming
// Responses result as a `reasoning` output item (with a summary_text), prepended
// before the assistant message, without leaking into the answer (M324).
func TestResponses_NonStreaming_ReasoningItem(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "42", reasoning: []string{"6*7 ", "= 42."}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","input":"6*7?"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Summary []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"summary"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		OutputText string `json:"output_text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Output) != 2 {
		t.Fatalf("expected reasoning + message items, got %+v", out.Output)
	}
	if out.Output[0].Type != "reasoning" || len(out.Output[0].Summary) != 1 ||
		out.Output[0].Summary[0].Type != "summary_text" || out.Output[0].Summary[0].Text != "6*7 = 42." {
		t.Errorf("reasoning item = %+v", out.Output[0])
	}
	if out.Output[1].Type != "message" || len(out.Output[1].Content) != 1 || out.Output[1].Content[0].Text != "42" {
		t.Errorf("message item = %+v", out.Output[1])
	}
	if out.OutputText != "42" {
		t.Errorf("output_text=%q (reasoning must not leak into the answer)", out.OutputText)
	}
}

// A non-reasoning run must carry exactly one (message) output item — no reasoning
// item — so the response stays byte-identical to before (M324).
func TestResponses_NonStreaming_NoReasoningItemWhenAbsent(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hi"}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","input":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), `"type":"reasoning"`) {
		t.Errorf("no reasoning item expected for a non-reasoning run:\n%s", rec.Body.String())
	}
}

// In streaming mode, reasoning must arrive as response.reasoning_summary_text
// .delta/.done events and appear as a reasoning output item in the final
// response.completed object (M324).
func TestResponses_Streaming_Reasoning(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "42", tokens: []string{"42"}, reasoning: []string{"6*7 ", "= 42."}}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"m","stream":true,"input":"6*7?"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	out := rec.Body.String()
	for _, want := range []string{
		"event: response.reasoning_summary_text.delta",
		`"delta":"6*7 "`,
		`"delta":"= 42."`,
		"event: response.reasoning_summary_text.done",
		`"type":"reasoning"`,
		`"type":"summary_text"`,
		`"text":"6*7 = 42."`,
		`"delta":"42"`, // the answer still streams as output_text
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stream missing %q in:\n%s", want, out)
		}
	}
}

func TestResponses_StreamingNonStreamProviderEmitsAnswerOnce(t *testing.T) {
	// No tokens streamed → the answer should arrive as a single delta.
	eng := &fakeEngine{model: "m", answer: "single shot"}
	s := newAPIServer(t, eng, "secret")
	body := `{"stream":true,"input":"q"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	out := rec.Body.String()
	if !strings.Contains(out, `"delta":"single shot"`) {
		t.Errorf("non-streaming answer should be emitted as one delta:\n%s", out)
	}
	if !strings.Contains(out, `"text":"single shot"`) {
		t.Errorf("output_text.done should carry the full answer:\n%s", out)
	}
}

func TestResponses_RejectsGET(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "secret")
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET should be 405, got %d", rec.Code)
	}
}

func TestResponses_EmptyInput(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "secret")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":""}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty input should be 400, got %d", rec.Code)
	}
}

func TestResponses_AuthRequired(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newAPIServer(t, eng, "secret")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hi"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no token should be 401, got %d", rec.Code)
	}
}

// TestResponses_JSONMode (M315): the Responses API honours structured output via
// either text.format.type or a top-level response_format; absence leaves it off.
func TestResponses_JSONMode(t *testing.T) {
	post := func(body string) *fakeEngine {
		eng := &fakeEngine{model: "m", answer: "{}"}
		s := newAPIServer(t, eng, "secret")
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		return eng
	}

	if e := post(`{"model":"m","input":"x","text":{"format":{"type":"json_object"}}}`); !e.ranJSONMode {
		t.Error("text.format json_object should set JSON mode")
	}
	if e := post(`{"model":"m","input":"x","response_format":{"type":"json_schema"}}`); !e.ranJSONMode {
		t.Error("top-level response_format json_schema should set JSON mode")
	}
	if e := post(`{"model":"m","input":"x"}`); e.ranJSONMode {
		t.Error("no format → JSON mode must stay off")
	}
}
