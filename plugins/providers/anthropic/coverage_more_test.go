// SPDX-License-Identifier: MIT

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestAnthropicCoverageIdentityEndpointParamsAndErrors(t *testing.T) {
	p := New("k")
	if p.Name() != "anthropic" {
		t.Fatalf("Name = %q", p.Name())
	}
	if got := (&APIError{Status: 529, Body: "overloaded"}).Error(); !strings.Contains(got, "529") || !strings.Contains(got, "overloaded") {
		t.Fatalf("APIError = %q", got)
	}
	p.Endpoint = "https://direct.example/messages"
	p.BaseURL = "https://ignored.example/v1"
	if got := p.resolveEndpoint(); got != "https://direct.example/messages" {
		t.Fatalf("explicit endpoint = %q", got)
	}
	p.Endpoint = ""
	p.BaseURL = "https://proxy.example/anthropic/v1/"
	if got := p.resolveEndpoint(); got != "https://proxy.example/anthropic/v1/messages" {
		t.Fatalf("base endpoint = %q", got)
	}
	if got := (&Provider{}).resolveEndpoint(); got != DefaultEndpoint {
		t.Fatalf("default endpoint = %q", got)
	}

	temp := 0.2
	topP := 0.8
	topK := 40
	wire := anthRequest{}
	wire.applyParams(agent.Params{Temperature: &temp, TopP: &topP, TopK: &topK, Stop: []string{"END"}})
	if wire.Temperature != &temp || wire.TopP != &topP || wire.TopK != &topK || len(wire.StopSequences) != 1 {
		t.Fatalf("applied params = %+v", wire)
	}
}

func TestAnthropicCoverageCanonicalAndEncodeBranches(t *testing.T) {
	if msg, err := canonicalToAnth(agent.Message{Role: agent.RoleSystem, Content: "ignored"}, nil); err != nil || msg != nil {
		t.Fatalf("system canonical = %#v err %v", msg, err)
	}
	assistant, err := canonicalToAnth(agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "tool"}}}, map[string]string{"tool": "wire_tool"})
	if err != nil {
		t.Fatalf("assistant canonical: %v", err)
	}
	if assistant.Role != "assistant" || len(assistant.Content) != 1 || assistant.Content[0].Name != "wire_tool" || string(assistant.Content[0].Input) != "{}" {
		t.Fatalf("assistant canonical = %+v", assistant)
	}
	if _, err := canonicalToAnth(agent.Message{Role: agent.RoleTool, Content: "out"}, nil); err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Fatalf("tool without id = %v", err)
	}
	if _, err := canonicalToAnth(agent.Message{Role: "alien", Content: "x"}, nil); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("unknown role = %v", err)
	}

	body, err := encodeRequest("claude", "system", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 128, 0, agent.Params{}, json.RawMessage(`{"metadata":{"test":true}}`))
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	text := string(body)
	for _, want := range []string{"cache_control", "plain", `"metadata":{"test":true}`} {
		if !strings.Contains(text, want) {
			t.Fatalf("encoded request missing %q in %s", want, text)
		}
	}
}

func TestAnthropicCoverageDecodeBranches(t *testing.T) {
	if _, err := decodeResponse([]byte(`not-json`)); err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("bad json decode = %v", err)
	}
	resp, err := decodeResponse([]byte(`{"model":"claude","stop_reason":"tool_use","content":[{"type":"thinking","thinking":"reason"},{"type":"text","text":"answer"},{"type":"tool_use","id":"call-1","name":"lookup"}],"usage":{"input_tokens":1,"cache_read_input_tokens":2,"cache_creation_input_tokens":3,"output_tokens":4}}`))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.StopReason != agent.StopToolUse || resp.Message.Content != "answer" || resp.ReasoningContent != "reason" || len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != "{}" {
		t.Fatalf("decoded response = %+v", resp)
	}
	if resp.Usage.InputTokens != 6 || resp.Usage.CachedInputTokens != 2 || resp.Usage.CacheWriteInputTokens != 3 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}
