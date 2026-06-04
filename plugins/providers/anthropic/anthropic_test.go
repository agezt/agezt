// SPDX-License-Identifier: MIT

package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestComplete_NoAPIKey(t *testing.T) {
	p := New("")
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("got err=%v, want ErrNoAPIKey", err)
	}
}

func TestComplete_HappyPath_TextOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate headers.
		if r.Header.Get("anthropic-version") != APIVersion {
			t.Errorf("missing/wrong anthropic-version: %q", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("wrong x-api-key: %q", r.Header.Get("x-api-key"))
		}
		// Inspect request body.
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"test-model"`) {
			t.Errorf("body missing model: %s", body)
		}
		if !strings.Contains(string(body), `"system":"sys"`) {
			t.Errorf("body missing system: %s", body)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{
			"id":"msg_x","type":"message","role":"assistant","model":"test-model",
			"content":[{"type":"text","text":"hello back"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":3,"output_tokens":2}
		}`))
	}))
	defer srv.Close()

	p := New("test-key")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "test-model",
		System:   "sys",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hello back" {
		t.Errorf("Content=%q want %q", resp.Message.Content, "hello back")
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason=%q want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Errorf("usage=%+v", resp.Usage)
	}
}

func TestComplete_ToolUseResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{
			"id":"msg_x","type":"message","role":"assistant","model":"m",
			"content":[
				{"type":"text","text":"I'll run shell."},
				{"type":"tool_use","id":"call_1","name":"shell","input":{"command":"ls"}}
			],
			"stop_reason":"tool_use",
			"usage":{"input_tokens":5,"output_tokens":7}
		}`))
	}))
	defer srv.Close()

	p := New("k")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "list files"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("StopReason=%q want tool_use", resp.StopReason)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls=%d want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "shell" {
		t.Errorf("ToolCall=%+v", tc)
	}
	if !strings.Contains(string(tc.Input), `"command":"ls"`) {
		t.Errorf("ToolCall.Input=%s", tc.Input)
	}
	if resp.Message.Content != "I'll run shell." {
		t.Errorf("text concat = %q", resp.Message.Content)
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer srv.Close()

	p := New("k")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error for 429")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got err=%v, want *APIError", err)
	}
	if apiErr.Status != 429 || !strings.Contains(apiErr.Body, "rate_limit") {
		t.Errorf("apiErr=%+v", apiErr)
	}
}

func TestEncodeRequest_TranslatesRoles(t *testing.T) {
	body, err := encodeRequest("m", "", []agent.Message{
		{Role: agent.RoleSystem, Content: "ignored"}, // routed to top-level field by caller; encoder skips
		{Role: agent.RoleUser, Content: "user1"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "shell", Input: json.RawMessage(`{"command":"ls"}`)}}},
		{Role: agent.RoleTool, ToolCallID: "c1", Content: "file1\nfile2"},
		{Role: agent.RoleAssistant, Content: "done"},
	}, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// System message must NOT appear as a message; SystemField is empty here so absent.
	if strings.Contains(s, `"text":"ignored"`) {
		t.Errorf("system message leaked into messages array: %s", s)
	}
	// Tool result must be wrapped in a user-role tool_result block.
	if !strings.Contains(s, `"tool_use_id":"c1"`) {
		t.Errorf("tool result missing tool_use_id: %s", s)
	}
	if !strings.Contains(s, `"content":"file1\nfile2"`) {
		t.Errorf("tool result body missing: %s", s)
	}
	// Tool-use block emitted on the assistant turn.
	if !strings.Contains(s, `"type":"tool_use"`) || !strings.Contains(s, `"name":"shell"`) {
		t.Errorf("tool_use block missing: %s", s)
	}
}

func TestEncodeRequest_SystemFieldRespected(t *testing.T) {
	body, _ := encodeRequest("m", "you are precise", []agent.Message{
		{Role: agent.RoleUser, Content: "hi"},
	}, nil, 100)
	if !strings.Contains(string(body), `"system":"you are precise"`) {
		t.Errorf("missing system field: %s", body)
	}
}

func TestDecodeResponse_MapsStopReasons(t *testing.T) {
	cases := map[string]agent.StopReason{
		"end_turn":      agent.StopEndTurn,
		"stop_sequence": agent.StopEndTurn,
		"tool_use":      agent.StopToolUse,
		"max_tokens":    agent.StopMaxTokens,
	}
	for in, want := range cases {
		raw := []byte(`{"id":"x","role":"assistant","content":[{"type":"text","text":""}],"stop_reason":"` + in + `","usage":{}}`)
		got, err := decodeResponse(raw)
		if err != nil {
			t.Fatalf("decode %s: %v", in, err)
		}
		if got.StopReason != want {
			t.Errorf("stop_reason %q → %q want %q", in, got.StopReason, want)
		}
	}
}

// TestDecodeResponse_CacheUsage verifies M290 cache-token accounting: Anthropic
// reports input_tokens EXCLUDING cached prompt tokens, so the canonical Usage
// must sum input + cache_read + cache_creation and mark cache_read as cached.
func TestDecodeResponse_CacheUsage(t *testing.T) {
	raw := []byte(`{"id":"x","role":"assistant","model":"m",
		"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn",
		"usage":{"input_tokens":100,"output_tokens":20,
		"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}`)
	got, err := decodeResponse(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Total prompt = 100 fresh + 900 read + 50 creation = 1050.
	if got.Usage.InputTokens != 1050 {
		t.Errorf("InputTokens=%d want 1050 (fresh+read+creation)", got.Usage.InputTokens)
	}
	if got.Usage.CachedInputTokens != 900 {
		t.Errorf("CachedInputTokens=%d want 900 (cache_read)", got.Usage.CachedInputTokens)
	}
	if got.Usage.CacheWriteInputTokens != 50 {
		t.Errorf("CacheWriteInputTokens=%d want 50 (cache_creation)", got.Usage.CacheWriteInputTokens)
	}
	if got.Usage.OutputTokens != 20 {
		t.Errorf("OutputTokens=%d want 20", got.Usage.OutputTokens)
	}
}

// TestEncodeRequest_PromptCacheMarksLastTool covers M299: the request marks the
// LAST tool definition with cache_control: ephemeral so Anthropic caches the
// stable tools prefix; tools before it carry no marker, and a tool-less request
// has none at all.
func TestEncodeRequest_PromptCacheMarksLastTool(t *testing.T) {
	var seen map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&seen)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x", "role": "assistant",
			"content": []map[string]any{{"type": "text", "text": "ok"}}, "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	p := New("k")
	p.Endpoint = srv.URL
	tools := []agent.ToolDef{
		{Name: "first", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "last", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
		Tools:    tools,
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	arr, _ := seen["tools"].([]any)
	if len(arr) != 2 {
		t.Fatalf("tools len=%d want 2", len(arr))
	}
	first, _ := arr[0].(map[string]any)
	if _, has := first["cache_control"]; has {
		t.Errorf("first tool must NOT carry cache_control")
	}
	last, _ := arr[1].(map[string]any)
	cc, ok := last["cache_control"].(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Errorf("last tool cache_control = %v want {type: ephemeral}", last["cache_control"])
	}
}
