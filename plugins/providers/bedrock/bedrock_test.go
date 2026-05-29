// SPDX-License-Identifier: MIT

package bedrock_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/bedrock"
)

func TestComplete_AnthropicOnBedrockTextResponse(t *testing.T) {
	var seen struct {
		path, auth string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_xyz", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "hi from bedrock"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 4, "output_tokens": 3},
		})
	}))
	defer srv.Close()

	p := bedrock.New("br-token", "us-east-1")
	p.Endpoint = srv.URL + "/model/anthropic.claude-opus-4-7/invoke"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "anthropic.claude-opus-4-7",
		System:   "be terse",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from bedrock" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop=%q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage=%+v", resp.Usage)
	}
	if seen.auth != "Bearer br-token" {
		t.Errorf("auth=%q", seen.auth)
	}
	// Body MUST carry anthropic_version and NOT carry a `model` field
	// (Bedrock puts the model id in the URL path).
	if seen.body["anthropic_version"] != "bedrock-2023-05-31" {
		t.Errorf("anthropic_version=%v", seen.body["anthropic_version"])
	}
	if _, hasModel := seen.body["model"]; hasModel {
		t.Errorf("body should NOT carry `model` field; got %v", seen.body["model"])
	}
	if seen.body["system"] != "be terse" {
		t.Errorf("system=%v", seen.body["system"])
	}
}

func TestComplete_ToolUseRoundtrip(t *testing.T) {
	var bodyOnWire map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&bodyOnWire)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_2", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "done"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	p := bedrock.New("k", "us-east-1")
	p.Endpoint = srv.URL + "/model/anthropic.claude-opus-4-7/invoke"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "anthropic.claude-opus-4-7",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "list"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{
				ID: "tu_1", Name: "shell", Input: json.RawMessage(`{"command":"ls"}`),
			}}},
			{Role: agent.RoleTool, ToolCallID: "tu_1", Content: "a.txt"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	msgs, _ := bodyOnWire["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages=%d want 3", len(msgs))
	}
	// Tool result is folded into the third (user-role) message as a
	// tool_result content block — Anthropic convention.
	last, _ := msgs[2].(map[string]any)
	if last["role"] != "user" {
		t.Errorf("tool-result message role=%v want user", last["role"])
	}
	parts, _ := last["content"].([]any)
	p0, _ := parts[0].(map[string]any)
	if p0["type"] != "tool_result" || p0["tool_use_id"] != "tu_1" {
		t.Errorf("tool_result block: %#v", p0)
	}
}

func TestComplete_ToolUseStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_3", "type": "message", "role": "assistant",
			"content": []map[string]any{
				{"type": "tool_use", "id": "tu_call", "name": "shell", "input": map[string]any{"command": "ls"}},
			},
			"stop_reason": "tool_use",
		})
	}))
	defer srv.Close()

	p := bedrock.New("k", "us-east-1")
	p.Endpoint = srv.URL + "/x"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "anthropic.claude-opus-4-7",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "list"}},
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
	if resp.Message.ToolCalls[0].ID != "tu_call" {
		t.Errorf("id=%q", resp.Message.ToolCalls[0].ID)
	}
}

func TestComplete_UnsupportedVendorRefused(t *testing.T) {
	// Amazon Titan / AI21 aren't wired (anthropic, mistral, cohere,
	// meta/llama are). The error must carry ErrVendorUnsupported so
	// callers can distinguish "this model needs a different body
	// shape" from generic API errors.
	p := bedrock.New("k", "us-east-1")
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "amazon.titan-text-express-v1",
	})
	if !errors.Is(err, bedrock.ErrVendorUnsupported) {
		t.Fatalf("got %v want ErrVendorUnsupported", err)
	}
}

func TestComplete_RegionalAnthropicProfileAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_r", "type": "message", "role": "assistant",
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	p := bedrock.New("k", "us-east-1")
	p.Endpoint = srv.URL + "/x"
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}); err != nil {
		t.Fatalf("regional profile should be accepted: %v", err)
	}
}

func TestComplete_NoBearerToken(t *testing.T) {
	p := bedrock.New("", "us-east-1")
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "anthropic.claude-opus-4-7",
	})
	if !errors.Is(err, bedrock.ErrNoBearerToken) {
		t.Errorf("got %v want ErrNoBearerToken", err)
	}
}

func TestResolveEndpoint(t *testing.T) {
	cases := []struct {
		name, endpoint, baseURL, region, model, want string
	}{
		{
			name:   "default — derive from region",
			region: "eu-central-1",
			model:  "anthropic.claude-opus-4-7",
			want:   "https://bedrock-runtime.eu-central-1.amazonaws.com/model/anthropic.claude-opus-4-7/invoke",
		},
		{
			name:    "custom.json BaseURL override",
			baseURL: "https://my-vpce-endpoint.internal",
			region:  "us-east-1", // ignored when BaseURL is set
			model:   "anthropic.claude-opus-4-7",
			want:    "https://my-vpce-endpoint.internal/model/anthropic.claude-opus-4-7/invoke",
		},
		{
			name:     "explicit full Endpoint wins",
			endpoint: "https://wherever.example/some/path",
			region:   "us-east-1",
			model:    "anthropic.claude-opus-4-7",
			want:     "https://wherever.example/some/path",
		},
		{
			name:    "trailing slash on BaseURL is trimmed",
			baseURL: "https://example.com/",
			region:  "us-east-1",
			model:   "anthropic.claude-opus-4-7",
			want:    "https://example.com/model/anthropic.claude-opus-4-7/invoke",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &bedrock.Provider{
				BearerToken: "k",
				Endpoint:    c.endpoint,
				BaseURL:     c.baseURL,
				Region:      c.region,
			}
			if got := p.ResolveEndpoint(c.model); got != c.want {
				t.Errorf("ResolveEndpoint=%q want %q", got, c.want)
			}
		})
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer srv.Close()

	p := bedrock.New("k", "us-east-1")
	p.Endpoint = srv.URL + "/x"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "anthropic.claude-opus-4-7",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	apiErr, ok := err.(*bedrock.APIError)
	if !ok {
		t.Fatalf("got %v want *bedrock.APIError", err)
	}
	if apiErr.Status != 403 {
		t.Errorf("status=%d", apiErr.Status)
	}
}
