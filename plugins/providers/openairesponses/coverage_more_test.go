// SPDX-License-Identifier: MIT

package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestOpenAIResponsesCoverageBasicProviderBranches(t *testing.T) {
	p := New("chatgpt", "gpt-5-codex", staticToken)
	if p.Name() != "chatgpt" {
		t.Fatalf("Name = %q", p.Name())
	}
	p.BaseURL = "https://example.com/base/"
	if got := p.base(); got != "https://example.com/base" {
		t.Fatalf("base trim = %q", got)
	}
	p.BaseURL = ""
	if got := p.base(); got != DefaultBaseURL {
		t.Fatalf("base default = %q", got)
	}
	p.newSession = func() string { return "fixed-session" }
	if got := p.session(); got != "fixed-session" {
		t.Fatalf("session override = %q", got)
	}

	if _, err := (&Provider{}).Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err == nil || !strings.Contains(err.Error(), "no token") {
		t.Fatalf("missing token error = %v", err)
	}
	if _, err := (&Provider{Token: staticToken}).Complete(context.Background(), agent.CompletionRequest{}); err == nil || !strings.Contains(err.Error(), "no model") {
		t.Fatalf("missing model error = %v", err)
	}
}

func TestOpenAIResponsesCoverageSendTokenErrorAndNoAccountHeader(t *testing.T) {
	p := New("chatgpt", "m", func(context.Context, bool) (string, string, error) {
		return "", "", errors.New("token unavailable")
	})
	if _, _, err := p.send(context.Background(), []byte(`{}`), true); err == nil || !strings.Contains(err.Error(), "token unavailable") {
		t.Fatalf("send token error = %v", err)
	}

	withLoopbackClient(t)
	var accountHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountHeader = r.Header.Get("chatgpt-account-id")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	p = New("chatgpt", "m", func(context.Context, bool) (string, string, error) { return "access", "", nil })
	p.BaseURL = srv.URL
	_, status, err := p.send(context.Background(), []byte(`{}`), false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("send = status %d err %v", status, err)
	}
	if accountHeader != "" {
		t.Fatalf("empty account id should omit header, got %q", accountHeader)
	}
}

func TestOpenAIResponsesCoverageToInputAllRoles(t *testing.T) {
	items := toInput([]agent.Message{
		{Role: agent.RoleSystem, Content: "system note"},
		{Role: agent.RoleUser, Content: "hello"},
		{Role: agent.RoleAssistant, Content: "assistant text", ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "lookup", Input: json.RawMessage(`{"q":"x"}`)}}},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-empty", Name: "noop"}}},
		{Role: agent.RoleTool, ToolCallID: "call-1", Content: "tool output"},
	})
	b, _ := json.Marshal(items)
	text := string(b)
	for _, want := range []string{"developer", "input_text", "output_text", "function_call", "lookup", `\"q\":\"x\"`, "call-empty", "{}", "function_call_output", "tool output"} {
		if !strings.Contains(text, want) {
			t.Fatalf("toInput missing %q in %s", want, text)
		}
	}
}

func TestOpenAIResponsesCoverageBuildBodyOptionsAndTools(t *testing.T) {
	temp := 0.25
	topP := 0.75
	p := New("chatgpt", "default", staticToken)
	p.ReasoningEffort = ""
	body, err := p.buildBody(agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
		Tools:    []agent.ToolDef{{Name: "plain_tool"}},
		Params:   agent.Params{Temperature: &temp, TopP: &topP, ReasoningEffort: "high"},
		ProviderOptions: map[string]json.RawMessage{
			"openai": json.RawMessage(`{"metadata":{"source":"test"}}`),
		},
	}, "model-x")
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body json: %v\n%s", err, body)
	}
	if got["tool_choice"] != "auto" || got["temperature"] != temp || got["top_p"] != topP {
		t.Fatalf("body knobs = %s", body)
	}
	if got["metadata"] == nil {
		t.Fatalf("provider options were not merged: %s", body)
	}
	reasoning, _ := got["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("reasoning = %#v", reasoning)
	}
	tools, _ := got["tools"].([]any)
	if len(tools) != 1 || !strings.Contains(string(body), `"parameters":{"type":"object"}`) {
		t.Fatalf("default tool parameters missing: %s", body)
	}
}

func TestOpenAIResponsesCoverageParseSSEFallbacksAndErrors(t *testing.T) {
	fallback := []byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"cached_input_tokens\":1},\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"fallback text\"}]}]}}\n\n")
	resp, err := parseSSE(fallback)
	if err != nil {
		t.Fatalf("parse fallback: %v", err)
	}
	if resp.Message.Content != "fallback text" || resp.Usage.CachedInputTokens != 1 {
		t.Fatalf("fallback response = %+v", resp)
	}

	deltaOnly := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n")
	resp, err = parseSSE(deltaOnly)
	if err != nil || resp.Message.Content != "hello world" {
		t.Fatalf("delta parse resp=%+v err=%v", resp, err)
	}

	failed := []byte("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"bad things\"}}}\n\n")
	if _, err := parseSSE(failed); err == nil || !strings.Contains(err.Error(), "bad things") {
		t.Fatalf("failed SSE error = %v", err)
	}
	errorEvent := sseEvent{Error: json.RawMessage(`{"message":"event failed"}`)}
	if got := sseError(errorEvent); got != "event failed" {
		t.Fatalf("sseError event = %q", got)
	}
	if got := sseError(sseEvent{}); got != "response failed" {
		t.Fatalf("sseError default = %q", got)
	}
}
