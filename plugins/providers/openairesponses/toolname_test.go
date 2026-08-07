// SPDX-License-Identifier: MIT

package openairesponses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// respNamePattern is what the Responses backend validates function names against.
var respNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// TestToolNamesConformToResponsesPattern is the regression for the live bug: a
// dotted tool name like "browser.read" was sent verbatim and the ChatGPT backend
// rejected the whole request with
// 400 "Invalid 'tools[N].name': string does not match pattern", killing this
// provider's arm of the routing chain. Every name on the wire — in the tools
// array AND in replayed function_call items — must now conform.
func TestToolNamesConformToResponsesPattern(t *testing.T) {
	p := New("chatgpt", "gpt-5.6-sol", staticToken)
	tools := []agent.ToolDef{
		{Name: "browser.read", Description: "fetch a page", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "shell", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "mcp:github/list issues", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	body, err := p.buildBody(agent.CompletionRequest{
		Tools: tools,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "go"},
			{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{
				{ID: "c1", Name: "browser.read", Input: json.RawMessage(`{"url":"x"}`)},
			}},
			{Role: agent.RoleTool, ToolCallID: "c1", Content: "ok"},
		},
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	if strings.Contains(string(body), "browser.read") {
		t.Fatalf("raw 'browser.read' leaked onto the wire: %s", body)
	}

	var req struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Input []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(req.Tools))
	}
	for _, tl := range req.Tools {
		if !respNamePattern.MatchString(tl.Name) {
			t.Errorf("tool name %q violates the Responses pattern", tl.Name)
		}
	}
	if req.Tools[0].Name != "browser_read" {
		t.Errorf("browser.read should wire to browser_read, got %q", req.Tools[0].Name)
	}
	var sawCall bool
	for _, it := range req.Input {
		if it.Type != "function_call" {
			continue
		}
		sawCall = true
		if it.Name != "browser_read" {
			t.Errorf("replayed function_call name = %q, want browser_read", it.Name)
		}
	}
	if !sawCall {
		t.Error("no function_call item in the request input")
	}
}

// TestToolCallNamesRestored checks the return leg: the model answers with the
// wire name, and Complete must hand back the ORIGINAL so the call routes to the
// real tool instead of failing as unknown.
func TestToolCallNamesRestored(t *testing.T) {
	withLoopbackClient(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\"," +
			"\"name\":\"browser_read\",\"arguments\":\"{\\\"url\\\":\\\"x\\\"}\",\"call_id\":\"c1\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
	}))
	defer srv.Close()

	p := New("chatgpt", "gpt-5.6-sol", staticToken)
	p.BaseURL = srv.URL
	resp, err := p.Complete(t.Context(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
		Tools:    []agent.ToolDef{{Name: "browser.read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.Message.ToolCalls))
	}
	if got := resp.Message.ToolCalls[0].Name; got != "browser.read" {
		t.Errorf("tool call name = %q, want the original browser.read", got)
	}
}
