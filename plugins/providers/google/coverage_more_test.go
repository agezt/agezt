// SPDX-License-Identifier: MIT

package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestGoogleCoverageIdentityEndpointAndErrors(t *testing.T) {
	p := New("k")
	if p.Name() != "google" {
		t.Fatalf("Name = %q", p.Name())
	}
	if got := (&APIError{Status: 403, Body: "denied"}).Error(); !strings.Contains(got, "403") || !strings.Contains(got, "denied") {
		t.Fatalf("APIError = %q", got)
	}
	p.Endpoint = "https://direct.example/generate"
	if got := p.resolveEndpoint("gemini"); got != "https://direct.example/generate" {
		t.Fatalf("explicit endpoint = %q", got)
	}
	if got := (&Provider{BaseURL: "https://proxy.example/api/v1/custom"}).resolveEndpoint("gemini"); got != "https://proxy.example/api/v1/custom/models/gemini:generateContent" {
		t.Fatalf("embedded /v1 endpoint = %q", got)
	}
}

func TestGoogleCoverageCanonicalEncodeAndDecodeBranches(t *testing.T) {
	if c, err := canonicalToGemini(agent.Message{Role: agent.RoleSystem, Content: "ignored"}, nil); err != nil || c != nil {
		t.Fatalf("system canonical = %#v err %v", c, err)
	}
	assistant, err := canonicalToGemini(agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{Name: "tool"}}}, map[string]string{"tool": "wire_tool"})
	if err != nil {
		t.Fatalf("assistant canonical: %v", err)
	}
	if assistant.Role != "model" || len(assistant.Parts) != 1 || assistant.Parts[0].FunctionCall.Name != "wire_tool" || string(assistant.Parts[0].FunctionCall.Args) != "{}" {
		t.Fatalf("assistant canonical = %+v", assistant)
	}
	if _, err := canonicalToGemini(agent.Message{Role: agent.RoleTool, Content: "out"}, nil); err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Fatalf("tool without id = %v", err)
	}
	if _, err := canonicalToGemini(agent.Message{Role: "alien", Content: "x"}, nil); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("unknown role = %v", err)
	}

	body, err := encodeRequest("system", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 9, true, -1, agent.Params{}, json.RawMessage(`{"safetySettings":[{"category":"test"}]}`))
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	text := string(body)
	for _, want := range []string{"systemInstruction", "responseMimeType", "thinkingConfig", "functionDeclarations", "safetySettings"} {
		if !strings.Contains(text, want) {
			t.Fatalf("encoded request missing %q in %s", want, text)
		}
	}

	if _, err := decodeResponse([]byte(`not-json`), "m"); err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("bad json decode = %v", err)
	}
	if _, err := decodeResponse([]byte(`{"candidates":[]}`), "m"); err == nil || !strings.Contains(err.Error(), "no candidates") {
		t.Fatalf("no candidates decode = %v", err)
	}
	resp, err := decodeResponse([]byte(`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"thought":true,"text":"reason"},{"text":"answer"},{"functionCall":{"name":"lookup","args":{}}}]} }],"usageMetadata":{"promptTokenCount":2,"cachedContentTokenCount":1,"candidatesTokenCount":3,"thoughtsTokenCount":4}}`), "gemini")
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.StopReason != agent.StopToolUse || resp.Message.Content != "answer" || resp.ReasoningContent != "reason" || resp.Usage.InputTokens != 2 || resp.Usage.CachedInputTokens != 1 || resp.Usage.OutputTokens != 7 || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("decoded response = %+v", resp)
	}
}
