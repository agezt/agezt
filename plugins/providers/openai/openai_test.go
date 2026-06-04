// SPDX-License-Identifier: MIT

package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/openai"
)

func TestComplete_TextResponse(t *testing.T) {
	var seen struct {
		auth, ctype string
		body        map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.auth = r.Header.Get("Authorization")
		seen.ctype = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "cmpl-1",
			"object": "chat.completion",
			"model":  "gpt-4o-mini",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
	defer srv.Close()

	p := openai.New("sk-test")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gpt-4o-mini",
		System:   "you are terse",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop=%q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 1 {
		t.Errorf("usage=%+v", resp.Usage)
	}
	if seen.auth != "Bearer sk-test" {
		t.Errorf("auth=%q", seen.auth)
	}
	if seen.ctype != "application/json" {
		t.Errorf("ctype=%q", seen.ctype)
	}
	if seen.body["model"] != "gpt-4o-mini" {
		t.Errorf("body.model=%v", seen.body["model"])
	}
	msgs, _ := seen.body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages len=%d want 2 (system+user)", len(msgs))
	}
	if m0, _ := msgs[0].(map[string]any); m0["role"] != "system" {
		t.Errorf("first message role=%v want system", m0["role"])
	}
}

func TestComplete_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "cmpl-2",
			"object": "chat.completion",
			"model":  "gpt-4o-mini",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"id":   "call_abc",
						"type": "function",
						"function": map[string]any{
							"name":      "shell",
							"arguments": `{"command":"ls"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 7},
		})
	}))
	defer srv.Close()

	p := openai.New("sk-test")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "list files"}},
		Tools: []agent.ToolDef{{
			Name:        "shell",
			Description: "run a shell command",
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
		t.Fatalf("tool_calls=%d want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("tc.ID=%q", tc.ID)
	}
	if tc.Name != "shell" {
		t.Errorf("tc.Name=%q", tc.Name)
	}
	if !strings.Contains(string(tc.Input), `"command":"ls"`) {
		t.Errorf("tc.Input=%s", string(tc.Input))
	}
}

func TestComplete_RoundtripWithToolResult(t *testing.T) {
	var bodyOnWire map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&bodyOnWire)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "cmpl-3",
			"object":  "chat.completion",
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "done"}, "finish_reason": "stop"}},
		})
	}))
	defer srv.Close()

	p := openai.New("sk-test")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "list"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{
				ID: "call_abc", Name: "shell", Input: json.RawMessage(`{"command":"ls"}`),
			}}},
			{Role: agent.RoleTool, ToolCallID: "call_abc", Content: "a.txt\nb.txt"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	msgs, _ := bodyOnWire["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("wire messages=%d want 3", len(msgs))
	}
	// Verify assistant tool_calls.arguments is a JSON-encoded string.
	asst, _ := msgs[1].(map[string]any)
	tcs, _ := asst["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("asst.tool_calls len=%d", len(tcs))
	}
	tc0, _ := tcs[0].(map[string]any)
	fn, _ := tc0["function"].(map[string]any)
	if args, _ := fn["arguments"].(string); !strings.Contains(args, `"command":"ls"`) {
		t.Errorf("arguments not a JSON string: %#v", fn["arguments"])
	}
	// Verify tool role + tool_call_id round-trips.
	tool, _ := msgs[2].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_abc" {
		t.Errorf("tool message: %#v", tool)
	}
}

func TestResolveEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		baseURL  string
		want     string
	}{
		{"explicit endpoint wins", "https://x.example/v1/chat/completions", "https://ignored.example", "https://x.example/v1/chat/completions"},
		{"baseurl with /v1 suffix", "", "https://api.groq.com/openai/v1", "https://api.groq.com/openai/v1/chat/completions"},
		{"baseurl with /v1/ inside", "", "https://api.example.com/v1/", "https://api.example.com/v1/chat/completions"},
		{"baseurl without /v1", "", "https://api.example.com", "https://api.example.com/v1/chat/completions"},
		{"defaults", "", "", openai.DefaultEndpoint},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &openai.Provider{Endpoint: c.endpoint, BaseURL: c.baseURL}
			// resolveEndpoint is unexported; trigger it via a roundtrip
			// to a recording server and verify against c.want for the
			// baseurl cases. For explicit & default cases the URL won't
			// hit our server, so we just exercise the precedence via a
			// fake key that triggers ErrNoAPIKey before HTTP.
			if c.endpoint == "" && c.baseURL == "" {
				return // default case: no observable behaviour without network
			}
			// Use httptest so we can observe what URL is hit.
			hit := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit = r.URL.Path
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": ""}, "finish_reason": "stop"}},
				})
			}))
			defer srv.Close()
			// Rewrite c.want's scheme+host onto the test server.
			if c.endpoint != "" {
				p.Endpoint = srv.URL + extractPath(c.want)
			} else {
				p.BaseURL = srv.URL + extractBase(c.baseURL)
			}
			p.APIKey = "k"
			if _, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err != nil {
				t.Fatalf("Complete: %v", err)
			}
			wantPath := extractPath(c.want)
			if hit != wantPath {
				t.Errorf("hit=%q want %q", hit, wantPath)
			}
		})
	}
}

func extractPath(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return u
	}
	rest := u[i+3:]
	if j := strings.Index(rest, "/"); j >= 0 {
		return rest[j:]
	}
	return "/"
}

func extractBase(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return ""
	}
	rest := u[i+3:]
	if j := strings.Index(rest, "/"); j >= 0 {
		return rest[j:]
	}
	return ""
}

func TestComplete_NoAPIKey(t *testing.T) {
	p := openai.New("")
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"})
	if err != openai.ErrNoAPIKey {
		t.Errorf("got %v want ErrNoAPIKey", err)
	}
}

func TestComplete_CustomAuthHeader(t *testing.T) {
	// Azure-OpenAI uses `api-key: <raw>` instead of `Authorization: Bearer <raw>`.
	// Verify the same adapter works when AuthHeader/AuthScheme are set.
	var seen struct{ authz, apiKey string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.authz = r.Header.Get("Authorization")
		seen.apiKey = r.Header.Get("api-key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
		})
	}))
	defer srv.Close()

	p := openai.New("az-secret")
	p.Endpoint = srv.URL
	p.AuthHeader = "api-key"
	p.AuthScheme = "" // raw value, no Bearer prefix
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "deployment-name"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if seen.authz != "" {
		t.Errorf("Authorization header should be absent; got %q", seen.authz)
	}
	if seen.apiKey != "az-secret" {
		t.Errorf("api-key=%q want az-secret", seen.apiKey)
	}
}

func TestComplete_DefaultAuthIsBearer(t *testing.T) {
	// Regression guard: the default behaviour must remain Bearer auth
	// on the Authorization header — no field set means existing
	// callers see no change.
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
		})
	}))
	defer srv.Close()

	p := openai.New("sk")
	p.Endpoint = srv.URL
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if seen != "Bearer sk" {
		t.Errorf("Authorization=%q want 'Bearer sk'", seen)
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer srv.Close()

	p := openai.New("k")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"})
	var apiErr *openai.APIError
	if !errorsAs(err, &apiErr) {
		t.Fatalf("got %v want APIError", err)
	}
	if apiErr.Status != 429 {
		t.Errorf("status=%d", apiErr.Status)
	}
}

// errorsAs is a tiny shim so we don't need to import "errors" just for
// this one call (keeps the test file's imports lean).
func errorsAs(err error, target any) bool {
	switch t := target.(type) {
	case **openai.APIError:
		if e, ok := err.(*openai.APIError); ok {
			*t = e
			return true
		}
	}
	return false
}

// TestComplete_CapturesReasoningContent (M317): a DeepSeek-R1-style response with
// message.reasoning_content surfaces on CompletionResponse.ReasoningContent,
// separate from the answer.
func TestComplete_CapturesReasoningContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":              "assistant",
					"content":           "42",
					"reasoning_content": "The user asks for the answer; it is 42.",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	p := openai.New("sk-test")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "deepseek-reasoner",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "answer?"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.ReasoningContent != "The user asks for the answer; it is 42." {
		t.Errorf("ReasoningContent=%q", resp.ReasoningContent)
	}
	if resp.Message.Content != "42" {
		t.Errorf("Content=%q (reasoning must not leak into the answer)", resp.Message.Content)
	}
}
