// SPDX-License-Identifier: MIT

package cohere

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestCohereCoverageIdentityEndpointAndErrors(t *testing.T) {
	p := New("k")
	if p.Name() != "cohere" {
		t.Fatalf("Name = %q", p.Name())
	}
	if got := (&APIError{Status: 429, Body: "slow down"}).Error(); !strings.Contains(got, "429") || !strings.Contains(got, "slow down") {
		t.Fatalf("APIError = %q", got)
	}
	p.Endpoint = "https://direct.example/chat"
	p.BaseURL = "https://ignored.example"
	if got := p.resolveEndpoint(); got != "https://direct.example/chat" {
		t.Fatalf("explicit endpoint = %q", got)
	}
	if got := (&Provider{BaseURL: "https://proxy.example/api/v2/custom"}).resolveEndpoint(); got != "https://proxy.example/api/v2/custom/chat" {
		t.Fatalf("embedded /v2 endpoint = %q", got)
	}
}

func TestCohereCoverageCanonicalValidationAndDefaults(t *testing.T) {
	if msg, err := canonicalToCohere(agent.Message{Role: agent.RoleSystem, Content: "   "}, nil); err != nil || msg != nil {
		t.Fatalf("blank system = %#v err %v", msg, err)
	}
	if _, err := canonicalToCohere(agent.Message{Role: agent.RoleTool, Content: "out"}, nil); err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Fatalf("tool without id error = %v", err)
	}
	if _, err := canonicalToCohere(agent.Message{Role: "alien", Content: "x"}, nil); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("unknown role error = %v", err)
	}
	msg, err := canonicalToCohere(agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "tool"}}}, map[string]string{"tool": "wire_tool"})
	if err != nil {
		t.Fatalf("assistant tool: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "wire_tool" || msg.ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("assistant tool message = %+v", msg)
	}
}

func TestCohereCoverageEncodeAndDecodeEdges(t *testing.T) {
	body, err := encodeRequest("command-r", "", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 0, agent.Params{}, json.RawMessage(`{"meta":"x"}`))
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	if !strings.Contains(string(body), `"parameters":{"type":"object","properties":{}}`) || !strings.Contains(string(body), `"meta":"x"`) {
		t.Fatalf("encoded body missing defaults/options: %s", body)
	}
	if _, err := decodeResponse([]byte(`not-json`), "m"); err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("bad json decode error = %v", err)
	}
	if _, err := decodeResponse([]byte(`{"message":{"content":123}}`), "m"); err == nil || !strings.Contains(err.Error(), "string-or-blocks") {
		t.Fatalf("bad content decode error = %v", err)
	}
	resp, err := decodeResponse([]byte(`{"finish_reason":"MAX_TOKENS","message":{"content":[{"type":"text","text":"partial"},{"type":"citation","text":"ignored"}],"tool_calls":[{"type":"function","function":{"name":"lookup","arguments":""}}]},"usage":{}}`), "command-r")
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens || resp.Message.Content != "partial" || len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != "{}" {
		t.Fatalf("decoded response = %+v", resp)
	}
	if resp.Message.ToolCalls[0].ID != "call-0" {
		t.Fatalf("default call id = %q", resp.Message.ToolCalls[0].ID)
	}
}
