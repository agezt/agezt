// SPDX-License-Identifier: MIT

package openairesponses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

func withLoopbackClient(t *testing.T) {
	t.Helper()
	prev := httpClientFor
	httpClientFor = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
	t.Cleanup(func() { httpClientFor = prev })
}

func staticToken(_ context.Context, _ bool) (string, string, error) { return "at-1", "acc-1", nil }

func TestCompleteTextAndUsage(t *testing.T) {
	withLoopbackClient(t)
	var gotBody map[string]any
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		// A message item, then a completed event with usage.
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello world\"}]}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := New("chatgpt", "gpt-5-codex", staticToken)
	p.BaseURL = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		System:   "be brief",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("content = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Fatalf("stop = %v", resp.StopReason)
	}
	// Headers + body shape.
	if gotHeaders.Get("Authorization") != "Bearer at-1" || gotHeaders.Get("chatgpt-account-id") != "acc-1" {
		t.Fatalf("auth headers = %v / %v", gotHeaders.Get("Authorization"), gotHeaders.Get("chatgpt-account-id"))
	}
	if gotHeaders.Get("originator") != originator || gotHeaders.Get("OpenAI-Beta") != betaHeader {
		t.Fatalf("codex headers missing: %v", gotHeaders)
	}
	instr, _ := gotBody["instructions"].(string)
	if !strings.HasPrefix(instr, "You are Codex") || !strings.Contains(instr, "be brief") {
		t.Fatalf("instructions should lead with Codex prompt then system: %.40q", instr)
	}
	if gotBody["stream"] != true || gotBody["store"] != false {
		t.Fatalf("stream/store = %v/%v", gotBody["stream"], gotBody["store"])
	}
}

func TestCompleteToolCall(t *testing.T) {
	withLoopbackClient(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"name\":\"get_weather\",\"arguments\":\"{\\\"city\\\":\\\"izmir\\\"}\",\"call_id\":\"call_1\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}}\n\n"))
	}))
	defer srv.Close()

	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)
	p := New("chatgpt", "gpt-5-codex", staticToken)
	p.BaseURL = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "weather?"}},
		Tools:    []agent.ToolDef{{Name: "get_weather", Description: "w", InputSchema: schema}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopToolUse || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message)
	}
	tc := resp.Message.ToolCalls[0]
	if tc.Name != "get_weather" || tc.ID != "call_1" || !strings.Contains(string(tc.Input), "izmir") {
		t.Fatalf("tool call = %+v", tc)
	}
	// Tools were forwarded as Responses function tools with tool_choice auto.
	if gotBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %v", gotBody["tool_choice"])
	}
	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", gotBody["tools"])
	}
}

func TestComplete401Refreshes(t *testing.T) {
	withLoopbackClient(t)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "Bearer stale" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"expired"}}`))
			return
		}
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{}}}\n\n"))
	}))
	defer srv.Close()

	var refreshed bool
	tok := func(_ context.Context, force bool) (string, string, error) {
		if force {
			refreshed = true
			return "fresh", "acc", nil
		}
		return "stale", "acc", nil
	}
	p := New("chatgpt", "gpt-5-codex", tok)
	p.BaseURL = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !refreshed || calls != 2 || resp.Message.Content != "ok" {
		t.Fatalf("expected 401→refresh→retry: refreshed=%v calls=%d content=%q", refreshed, calls, resp.Message.Content)
	}
}

func TestCompleteBackendError(t *testing.T) {
	withLoopbackClient(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer srv.Close()
	p := New("chatgpt", "gpt-5-codex", staticToken)
	p.BaseURL = srv.URL
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	}); err == nil {
		t.Fatal("expected error on 400")
	}
}
