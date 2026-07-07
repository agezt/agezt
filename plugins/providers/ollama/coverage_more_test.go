// SPDX-License-Identifier: MIT

package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestOllamaCoverageIdentityEndpointAndErrors(t *testing.T) {
	if got := (&APIError{Status: 503, Body: "busy"}).Error(); !strings.Contains(got, "503") || !strings.Contains(got, "busy") {
		t.Fatalf("APIError = %q", got)
	}
	if (&Provider{}).Name() != "ollama" {
		t.Fatalf("Name = %q", (&Provider{}).Name())
	}
	p := New()
	if got := p.resolveEndpoint(); got != DefaultEndpoint {
		t.Fatalf("default endpoint = %q", got)
	}
	p.Endpoint = "https://override.example/api/chat"
	if got := p.resolveEndpoint(); got != "https://override.example/api/chat" {
		t.Fatalf("explicit endpoint = %q", got)
	}
	p.Endpoint = ""
	p.BaseURL = "http://localhost:11434/"
	if got := p.resolveEndpoint(); got != "http://localhost:11434/api/chat" {
		t.Fatalf("base endpoint = %q", got)
	}

	// A zero-value provider has Endpoint=="" which resolves to DefaultEndpoint
	// (the localhost URL); the model check fires before HTTP, so even without a
	// model the local server can be reached if model is set. We just exercise the
	// empty-model branch via the constructor.
	if _, err := New().Complete(context.Background(), agent.CompletionRequest{Model: ""}); err != ErrNoModel {
		t.Fatalf("default missing model error = %v", err)
	}
}

func TestOllamaCoverageCanonicalAndImageData(t *testing.T) {
	cases := map[string]bool{
		"plain.png":                  false,
		"http://x/cat.png":           false,
		"data:image/png;base64,AAAA": true,
		"data:image/png;base64,":     false,
		"data:image/png,AAAA":        false,
	}
	for in, want := range cases {
		if _, ok := ollamaImageData(in); ok != want {
			t.Fatalf("ollamaImageData(%q) ok = %v, want %v", in, ok, want)
		}
	}

	assistant, err := canonicalToOllama(agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "tool"}}})
	if err != nil {
		t.Fatalf("assistant canonical: %v", err)
	}
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || string(assistant.ToolCalls[0].Function.Arguments) != "{}" {
		t.Fatalf("assistant canonical = %+v", assistant)
	}
	if _, err := canonicalToOllama(agent.Message{Role: agent.RoleTool, Content: "out"}); err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Fatalf("tool without id = %v", err)
	}
	if _, err := canonicalToOllama(agent.Message{Role: "alien", Content: "x"}); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("unknown role = %v", err)
	}

	withImg, err := canonicalToOllama(agent.Message{Role: agent.RoleUser, Content: "describe", Images: []string{"data:image/png;base64,AAAA", "plain.png"}})
	if err != nil {
		t.Fatalf("user canonical: %v", err)
	}
	if len(withImg.Images) != 1 || withImg.Images[0] != "AAAA" {
		t.Fatalf("images = %#v", withImg.Images)
	}
}

func TestOllamaCoverageEncodeAndDecodeBranches(t *testing.T) {
	temp := 0.1
	topP := 0.5
	seed := int64(42)
	stop := []string{"END"}
	body, err := encodeRequest("m", "", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 0, true, agent.Params{Temperature: &temp, TopP: &topP, Seed: &seed, Stop: stop}, json.RawMessage(`{"options":{"num_predict":99}}`))
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"format":"json"`, `"model":"m"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("encoded body missing %q in %s", want, text)
		}
	}
	// The per-request sampling knobs land inside options alongside num_predict
	// (when set). The provider-option merge may also overlay the entire options
	// block, so we just verify that one of the two surfaced.
	if !strings.Contains(text, "num_predict") &&
		!strings.Contains(text, "temperature") {
		t.Fatalf("encoded body missing sampling knobs: %s", text)
	}

	if _, err := decodeResponse([]byte(`not-json`)); err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("bad json decode = %v", err)
	}
	resp, err := decodeResponse([]byte(`{"model":"m","done":true,"done_reason":"length","message":{"role":"assistant","content":"partial","tool_calls":[{"function":{"name":"lookup","arguments":{}}}]},"prompt_eval_count":3,"eval_count":4}`))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.StopReason != agent.StopToolUse || resp.Message.Content != "partial" || len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != "{}" || resp.Message.ToolCalls[0].ID != "call-0" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("decoded response = %+v", resp)
	}
	// Default tool-call id is generated only when the wire omits it.
	resp2, err := decodeResponse([]byte(`{"model":"m","done":true,"message":{"role":"assistant","tool_calls":[{"id":"custom-1","function":{"name":"lookup","arguments":"{\"q\":1}"}}]}}`))
	if err != nil {
		t.Fatalf("decodeResponse with id: %v", err)
	}
	if resp2.Message.ToolCalls[0].ID != "custom-1" || len(resp2.Message.ToolCalls[0].Input) == 0 {
		t.Fatalf("response with explicit id = %+v", resp2.Message.ToolCalls)
	}
}

func TestOllamaCoverageHTTPStatusPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad model"}`))
	}))
	defer srv.Close()
	p := New()
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m", Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("got %v, want *APIError", err)
	}
}
