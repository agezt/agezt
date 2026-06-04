// SPDX-License-Identifier: MIT

package google_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/google"
)

func TestComplete_TextResponse(t *testing.T) {
	var seen struct {
		method, path, apiKey string
		body                 map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.apiKey = r.Header.Get("x-goog-api-key")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": "hi from gemini"}},
				},
				"finishReason": "STOP",
				"index":        0,
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount":     5,
				"candidatesTokenCount": 3,
				"totalTokenCount":      8,
			},
		})
	}))
	defer srv.Close()

	p := google.New("test-key")
	p.Endpoint = srv.URL + "/v1beta/models/gemini-1.5-flash:generateContent"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		System:   "be terse",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from gemini" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop=%q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage=%+v", resp.Usage)
	}
	if seen.method != "POST" {
		t.Errorf("method=%q", seen.method)
	}
	if seen.apiKey != "test-key" {
		t.Errorf("api key header=%q", seen.apiKey)
	}
	// systemInstruction should be at the top level, NOT in contents.
	si, ok := seen.body["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction missing: %#v", seen.body)
	}
	siParts, _ := si["parts"].([]any)
	if len(siParts) == 0 {
		t.Errorf("systemInstruction.parts empty")
	}
	contents, _ := seen.body["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("contents len=%d want 1 (user only; system folded to systemInstruction)", len(contents))
	}
	c0, _ := contents[0].(map[string]any)
	if c0["role"] != "user" {
		t.Errorf("first content role=%v want user", c0["role"])
	}
}

func TestComplete_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{{
						"functionCall": map[string]any{
							"name": "shell",
							"args": map[string]any{"command": "ls"},
						},
					}},
				},
				"finishReason": "STOP",
			}},
		})
	}))
	defer srv.Close()

	p := google.New("k")
	p.Endpoint = srv.URL + "/v1beta/models/gemini-1.5-pro:generateContent"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-pro",
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
		t.Fatalf("tool_calls len=%d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.Name != "shell" {
		t.Errorf("name=%q", tc.Name)
	}
	if tc.ID == "" {
		t.Error("synthesized id should not be empty")
	}
	if !strings.Contains(string(tc.Input), `"command":"ls"`) {
		t.Errorf("input=%s", string(tc.Input))
	}
}

func TestEncode_ToolDefsAsFunctionDeclarations(t *testing.T) {
	var bodyOnWire map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&bodyOnWire)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}},
				"finishReason": "STOP",
			}},
		})
	}))
	defer srv.Close()

	p := google.New("k")
	p.Endpoint = srv.URL + "/x"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
		Tools: []agent.ToolDef{
			{Name: "a", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "b", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	tools, _ := bodyOnWire["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools wrapper len=%d want 1 (all decls under one tools[0])", len(tools))
	}
	t0, _ := tools[0].(map[string]any)
	decls, _ := t0["functionDeclarations"].([]any)
	if len(decls) != 2 {
		t.Errorf("functionDeclarations len=%d want 2", len(decls))
	}
}

func TestEncode_ToolResultRoundtrip(t *testing.T) {
	var bodyOnWire map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&bodyOnWire)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "done"}}},
				"finishReason": "STOP",
			}},
		})
	}))
	defer srv.Close()

	p := google.New("k")
	p.Endpoint = srv.URL + "/x"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "gemini",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "list"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{
				ID: "call-0", Name: "shell", Input: json.RawMessage(`{"command":"ls"}`),
			}}},
			{Role: agent.RoleTool, ToolCallID: "call-0", Content: "a.txt\nb.txt"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	contents, _ := bodyOnWire["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("contents len=%d want 3", len(contents))
	}
	// Last content should be user-role with a functionResponse part.
	last, _ := contents[2].(map[string]any)
	if last["role"] != "user" {
		t.Errorf("tool-result role=%v want user", last["role"])
	}
	parts, _ := last["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("tool-result parts len=%d", len(parts))
	}
	p0, _ := parts[0].(map[string]any)
	fr, ok := p0["functionResponse"].(map[string]any)
	if !ok {
		t.Fatalf("tool-result part has no functionResponse: %#v", p0)
	}
	if fr["name"] == nil || fr["name"] == "" {
		t.Error("functionResponse.name empty")
	}
}

func TestResolveEndpoint(t *testing.T) {
	cases := []struct {
		name, base, model, wantSuffix string
	}{
		{"default base + model", "", "gemini-1.5-flash", "/v1beta/models/gemini-1.5-flash:generateContent"},
		{"explicit baseurl no version", "https://example.com", "g", "/v1beta/models/g:generateContent"},
		{"explicit baseurl with /v1beta", "https://example.com/v1beta", "g", "/v1beta/models/g:generateContent"},
		{"explicit baseurl with /v1", "https://example.com/v1", "g", "/v1/models/g:generateContent"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hit := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit = r.URL.Path
				_ = json.NewEncoder(w).Encode(map[string]any{
					"candidates": []map[string]any{{
						"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": ""}}},
						"finishReason": "STOP",
					}},
				})
			}))
			defer srv.Close()

			p := google.New("k")
			// Splice the test server URL into the requested base.
			if c.base == "" {
				p.BaseURL = srv.URL
			} else {
				// Replace scheme+host of c.base with the test server's.
				p.BaseURL = srv.URL + suffixAfterHost(c.base)
			}
			_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: c.model})
			if err != nil {
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
	p := google.New("")
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "g"}); err != google.ErrNoAPIKey {
		t.Errorf("got %v want ErrNoAPIKey", err)
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	p := google.New("k")
	p.Endpoint = srv.URL + "/x"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "g"})
	apiErr, ok := err.(*google.APIError)
	if !ok {
		t.Fatalf("got %v want *google.APIError", err)
	}
	if apiErr.Status != 400 {
		t.Errorf("status=%d", apiErr.Status)
	}
}

// TestComplete_CacheUsage covers M294-cache: Gemini's promptTokenCount includes
// the cached subset, surfaced separately as cachedContentTokenCount → mapped to
// Usage.CachedInputTokens (InputTokens stays the full prompt count).
func TestComplete_CacheUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount":        1000,
				"candidatesTokenCount":    20,
				"cachedContentTokenCount": 800,
			},
		})
	}))
	defer srv.Close()

	p := google.New("k")
	p.Endpoint = srv.URL + "/v1beta/models/gemini-1.5-flash:generateContent"
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Usage.InputTokens != 1000 {
		t.Errorf("InputTokens=%d want 1000", resp.Usage.InputTokens)
	}
	if resp.Usage.CachedInputTokens != 800 {
		t.Errorf("CachedInputTokens=%d want 800", resp.Usage.CachedInputTokens)
	}
}
