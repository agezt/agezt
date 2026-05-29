// SPDX-License-Identifier: MIT

package cohere_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/plugins/providers/cohere"
)

func TestComplete_TextResponseAsBlocks(t *testing.T) {
	var seen struct {
		path, auth string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg-1",
			"finish_reason": "COMPLETE",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "bonjour from cohere"}},
			},
			"usage": map[string]any{"tokens": map[string]any{"input_tokens": 5, "output_tokens": 4}},
		})
	}))
	defer srv.Close()

	p := cohere.New("co-key")
	p.Endpoint = srv.URL + "/v2/chat"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "command-r-plus",
		System:   "be terse",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "salut"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "bonjour from cohere" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop=%q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 4 {
		t.Errorf("usage=%+v", resp.Usage)
	}
	if seen.path != "/v2/chat" {
		t.Errorf("path=%q", seen.path)
	}
	if seen.auth != "Bearer co-key" {
		t.Errorf("auth=%q", seen.auth)
	}
	// system folded into the first message (system role), then user.
	msgs, _ := seen.body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages=%d want 2", len(msgs))
	}
	if m0, _ := msgs[0].(map[string]any); m0["role"] != "system" {
		t.Errorf("first message role=%v want system", m0["role"])
	}
}

func TestComplete_TextResponseAsString(t *testing.T) {
	// Some Cohere variants return content as a plain string. Adapter must tolerate both.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg-2",
			"finish_reason": "COMPLETE",
			"message": map[string]any{
				"role":    "assistant",
				"content": "plain string content",
			},
		})
	}))
	defer srv.Close()

	p := cohere.New("k")
	p.Endpoint = srv.URL + "/v2/chat"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "command-r",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "plain string content" {
		t.Errorf("content=%q", resp.Message.Content)
	}
}

func TestComplete_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg-3",
			"finish_reason": "TOOL_CALL",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{},
				"tool_calls": []map[string]any{{
					"id":   "tc_abc",
					"type": "function",
					"function": map[string]any{
						"name":      "shell",
						"arguments": `{"command":"ls"}`,
					},
				}},
			},
		})
	}))
	defer srv.Close()

	p := cohere.New("k")
	p.Endpoint = srv.URL + "/v2/chat"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "command-r",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "list"}},
		Tools: []agent.ToolDef{{
			Name: "shell", Description: "run shell",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop=%q want tool_use", resp.StopReason)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls=%d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "tc_abc" || tc.Name != "shell" {
		t.Errorf("toolCall={ID=%q Name=%q}", tc.ID, tc.Name)
	}
	if !strings.Contains(string(tc.Input), `"command":"ls"`) {
		t.Errorf("input=%s", string(tc.Input))
	}
}

func TestComplete_ToolResultRoundtrip(t *testing.T) {
	var bodyOnWire map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&bodyOnWire)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg-4",
			"finish_reason": "COMPLETE",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "done"}},
			},
		})
	}))
	defer srv.Close()

	p := cohere.New("k")
	p.Endpoint = srv.URL + "/v2/chat"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "command-r",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "list"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{
				ID: "tc_abc", Name: "shell", Input: json.RawMessage(`{"command":"ls"}`),
			}}},
			{Role: agent.RoleTool, ToolCallID: "tc_abc", Content: "a.txt\nb.txt"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	msgs, _ := bodyOnWire["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("wire messages=%d want 3", len(msgs))
	}
	tool, _ := msgs[2].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "tc_abc" {
		t.Errorf("tool message=%#v", tool)
	}
}

func TestResolveEndpoint(t *testing.T) {
	cases := []struct {
		name, base, wantSuffix string
	}{
		{"baseurl bare", "https://example.com", "/v2/chat"},
		{"baseurl with /v2", "https://example.com/v2", "/v2/chat"},
		{"baseurl with trailing slash", "https://example.com/", "/v2/chat"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hit := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit = r.URL.Path
				_ = json.NewEncoder(w).Encode(map[string]any{
					"finish_reason": "COMPLETE",
					"message":       map[string]any{"role": "assistant", "content": ""},
				})
			}))
			defer srv.Close()
			p := cohere.New("k")
			// Splice scheme+host of c.base with the test server.
			suffix := suffixAfterHost(c.base)
			p.BaseURL = srv.URL + suffix
			if _, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if hit != c.wantSuffix {
				t.Errorf("hit=%q want %q", hit, c.wantSuffix)
			}
		})
	}
}

func suffixAfterHost(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return u
	}
	rest := u[i+3:]
	if j := strings.Index(rest, "/"); j >= 0 {
		return rest[j:]
	}
	return ""
}

func TestComplete_NoAPIKey(t *testing.T) {
	p := cohere.New("")
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err != cohere.ErrNoAPIKey {
		t.Errorf("got %v want ErrNoAPIKey", err)
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	p := cohere.New("k")
	p.Endpoint = srv.URL + "/v2/chat"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"})
	apiErr, ok := err.(*cohere.APIError)
	if !ok {
		t.Fatalf("got %v want *cohere.APIError", err)
	}
	if apiErr.Status != 401 {
		t.Errorf("status=%d", apiErr.Status)
	}
}
