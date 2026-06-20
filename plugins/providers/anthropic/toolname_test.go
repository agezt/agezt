// SPDX-License-Identifier: MIT

package anthropic

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

var anthNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// TestToolNamesConformToAnthropicPattern is the regression for the live bug:
// a dotted tool name like "browser.read" used to be sent verbatim and Anthropic
// rejected the whole request with 400 invalid_request_error. Every tool name on
// the wire — in the tools array AND in assistant-history tool_use blocks — must
// now match ^[a-zA-Z0-9_-]{1,64}$.
func TestToolNamesConformToAnthropicPattern(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "browser.read", Description: "fetch a page", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "shell", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: "go"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "browser.read", Input: json.RawMessage(`{"url":"x"}`)}}},
		{Role: agent.RoleTool, ToolCallID: "c1", Content: "ok"},
	}
	body, err := encodeRequest("m", "", msgs, tools, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	// No raw dot survives anywhere in the request.
	if strings.Contains(string(body), "browser.read") {
		t.Fatalf("raw 'browser.read' leaked onto the wire: %s", body)
	}
	for _, tl := range req.Tools {
		if !anthNamePattern.MatchString(tl.Name) {
			t.Errorf("tool name %q violates Anthropic's pattern", tl.Name)
		}
	}
	if req.Tools[0].Name != "browser_read" {
		t.Errorf("browser.read should wire to browser_read, got %q", req.Tools[0].Name)
	}
	for _, m := range req.Messages {
		for _, c := range m.Content {
			if c.Type == "tool_use" && !anthNamePattern.MatchString(c.Name) {
				t.Errorf("assistant tool_use name %q violates the pattern", c.Name)
			}
		}
	}
}

// TestRestoreToolCallNames_RoundTrip verifies a tool_use returned under the wire
// name is mapped back to the original so the call routes to the real tool.
func TestRestoreToolCallNames_RoundTrip(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read"}}
	fwd, rev := wireToolNames(tools)
	if fwd["browser.read"] != "browser_read" || rev["browser_read"] != "browser.read" {
		t.Fatalf("maps wrong: fwd=%v rev=%v", fwd, rev)
	}
	resp := &agent.CompletionResponse{
		Message: agent.Message{ToolCalls: []agent.ToolCall{{ID: "1", Name: "browser_read"}}},
	}
	restoreToolCallNames(resp, reverseToolNames(tools))
	if resp.Message.ToolCalls[0].Name != "browser.read" {
		t.Fatalf("name not restored: %q", resp.Message.ToolCalls[0].Name)
	}
}

// TestWireToolNames_CollisionsAndLength: distinct names that sanitise to the same
// wire string get a deterministic suffix, and over-long names are capped at 64.
func TestWireToolNames_CollisionsAndLength(t *testing.T) {
	fwd, _ := wireToolNames([]agent.ToolDef{{Name: "browser.read"}, {Name: "browser_read"}})
	a, b := fwd["browser.read"], fwd["browser_read"]
	if a == b {
		t.Fatalf("collision not broken: both → %q", a)
	}
	if !anthNamePattern.MatchString(a) || !anthNamePattern.MatchString(b) {
		t.Fatalf("collision wire names invalid: %q %q", a, b)
	}

	long := strings.Repeat("a.b", 40) // 120 chars with dots
	fwd2, _ := wireToolNames([]agent.ToolDef{{Name: long}})
	w := fwd2[long]
	if len(w) > 64 || !anthNamePattern.MatchString(w) {
		t.Fatalf("long name not conformed: len=%d %q", len(w), w)
	}
}
