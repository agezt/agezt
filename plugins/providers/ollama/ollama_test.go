// SPDX-License-Identifier: MIT

package ollama

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

func TestComplete_HappyPath_TextOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"stream":false`) {
			t.Errorf("request must set stream=false; got %s", body)
		}
		if !strings.Contains(string(body), `"model":"llama-test"`) {
			t.Errorf("model missing: %s", body)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{
			"model":"llama-test","done":true,"done_reason":"stop",
			"message":{"role":"assistant","content":"hello back"},
			"prompt_eval_count":4,"eval_count":3
		}`))
	}))
	defer srv.Close()

	p := New()
	p.Endpoint = srv.URL
	p.Model = "llama-test"

	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hello back" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason=%q want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage=%+v", resp.Usage)
	}
}

func TestComplete_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{
			"model":"m","done":true,"done_reason":"stop",
			"message":{
				"role":"assistant","content":"running shell",
				"tool_calls":[
					{"function":{"name":"shell","arguments":{"command":"ls"}}}
				]
			}
		}`))
	}))
	defer srv.Close()

	p := New()
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "list"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("StopReason=%q want tool_use", resp.StopReason)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls=%d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.Name != "shell" {
		t.Errorf("name=%q", tc.Name)
	}
	if tc.ID == "" {
		t.Error("ToolCall.ID must be non-empty even when Ollama didn't supply one")
	}
	if !strings.Contains(string(tc.Input), `"command":"ls"`) {
		t.Errorf("Input=%s", tc.Input)
	}
}

func TestComplete_StopReasonLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"model":"m","done":true,"done_reason":"length","message":{"role":"assistant","content":"..."}}`))
	}))
	defer srv.Close()
	p := New()
	p.Endpoint = srv.URL
	resp, _ := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("StopReason=%q want max_tokens", resp.StopReason)
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()
	p := New()
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want *APIError", err)
	}
	if apiErr.Status != 404 {
		t.Errorf("status=%d", apiErr.Status)
	}
}

func TestEncodeRequest_RolesAndTools(t *testing.T) {
	body, err := encodeRequest("m", "be precise",
		[]agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "shell", Input: json.RawMessage(`{"command":"ls"}`)}}},
			{Role: agent.RoleTool, ToolCallID: "c1", Content: "file1\nfile2"},
		},
		[]agent.ToolDef{{Name: "shell", Description: "run a command", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		0,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `"role":"system"`) || !strings.Contains(s, `"content":"be precise"`) {
		t.Errorf("system prompt missing: %s", s)
	}
	if !strings.Contains(s, `"role":"tool"`) || !strings.Contains(s, `"tool_call_id":"c1"`) {
		t.Errorf("tool message missing: %s", s)
	}
	if !strings.Contains(s, `"type":"function"`) || !strings.Contains(s, `"name":"shell"`) {
		t.Errorf("tool def missing: %s", s)
	}
}

func TestRoleTool_RequiresID(t *testing.T) {
	_, err := canonicalToOllama(agent.Message{Role: agent.RoleTool, Content: "x"})
	if err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Errorf("expected tool_call_id error; got %v", err)
	}
}

// TestEncodeRequest_MaxTokensAsNumPredict (M310): the run's token cap is
// forwarded as Ollama's options.num_predict; 0 omits it (Ollama's own default).
func TestEncodeRequest_MaxTokensAsNumPredict(t *testing.T) {
	body, err := encodeRequest("llama3", "", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, 256, false)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Options map[string]any `json:"options"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.Options == nil || req.Options["num_predict"] == nil {
		t.Fatalf("num_predict missing from options: %s", body)
	}
	if n, _ := req.Options["num_predict"].(float64); n != 256 {
		t.Errorf("num_predict=%v want 256", req.Options["num_predict"])
	}

	// 0 → options omitted entirely (no behaviour change for uncapped runs).
	body0, _ := encodeRequest("llama3", "", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, 0, false)
	if strings.Contains(string(body0), "num_predict") {
		t.Errorf("maxTokens=0 must omit num_predict: %s", body0)
	}
}
