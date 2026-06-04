// SPDX-License-Identifier: MIT

package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// OpenAI requires tool names to match ^[a-zA-Z0-9_-]+$. Agezt's dotted names
// (browser.read) must be sanitised on the wire or the API 400s — which the mock
// fallback silently masked (M279).
func TestSanitizeToolName(t *testing.T) {
	cases := map[string]string{
		"browser.read": "browser_read",
		"shell":        "shell",
		"memory":       "memory",
		"a.b.c":        "a_b_c",
		"keep-dash_ok": "keep-dash_ok",
		"weird name!":  "weird_name_",
	}
	for in, want := range cases {
		if got := sanitizeToolName(in); got != want {
			t.Errorf("sanitizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The encoded request carries the sanitised name (matching the OpenAI pattern),
// never the raw dotted one.
func TestEncodeRequestSanitizesToolNames(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read", Description: "read a page"}}
	body, err := encodeRequest("gpt-5.5", "sys", nil, tools, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, `"browser.read"`) {
		t.Errorf("request leaks the dotted tool name: %s", s)
	}
	if !strings.Contains(s, `"browser_read"`) {
		t.Errorf("request missing the sanitised tool name: %s", s)
	}
}

// A tool_call the model returns under the sanitised name is mapped back to the
// original, so the kernel routes it to the real tool.
func TestRestoreToolCallNames(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read"}, {Name: "shell"}}
	rev := reverseToolNames(tools)
	if rev["browser_read"] != "browser.read" {
		t.Fatalf("reverse map = %v, want browser_read→browser.read", rev)
	}
	if _, ok := rev["shell"]; ok {
		t.Errorf("unchanged name should not be in the reverse map: %v", rev)
	}

	resp := &agent.CompletionResponse{Message: agent.Message{
		ToolCalls: []agent.ToolCall{
			{ID: "c1", Name: "browser_read", Input: json.RawMessage(`{"url":"x"}`)},
			{ID: "c2", Name: "shell", Input: json.RawMessage(`{}`)},
		},
	}}
	restoreToolCallNames(resp, rev)
	if resp.Message.ToolCalls[0].Name != "browser.read" {
		t.Errorf("tool call 0 name = %q, want browser.read", resp.Message.ToolCalls[0].Name)
	}
	if resp.Message.ToolCalls[1].Name != "shell" {
		t.Errorf("tool call 1 name = %q, want shell unchanged", resp.Message.ToolCalls[1].Name)
	}
}
