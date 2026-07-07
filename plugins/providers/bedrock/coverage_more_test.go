// SPDX-License-Identifier: MIT

package bedrock

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestBedrockCoverageIdentityErrorsHeadersAndFamilies(t *testing.T) {
	p := New("bearer", "us-east-1")
	if p.Name() != "bedrock" {
		t.Fatalf("Name = %q", p.Name())
	}
	if got := (&APIError{Status: 503, Body: "busy"}).Error(); !strings.Contains(got, "503") || !strings.Contains(got, "busy") {
		t.Fatalf("APIError = %q", got)
	}
	h := http.Header{}
	h.Set("X", " 42 ")
	if got := headerTokenCount(h, "X"); got != 42 {
		t.Fatalf("headerTokenCount = %d", got)
	}
	h.Set("X", "-1")
	if got := headerTokenCount(h, "X"); got != 0 {
		t.Fatalf("negative headerTokenCount = %d", got)
	}
	if !isCohereModel("us.cohere.command-r-v1:0") || isCohereModel("anthropic.claude") {
		t.Fatal("cohere family detection mismatch")
	}
	if !isMetaLlamaModel("eu.meta.llama3-70b") || isMetaLlamaModel("mistral.large") {
		t.Fatal("meta family detection mismatch")
	}
}

func TestBedrockCoverageAnthropicHelpers(t *testing.T) {
	if msg, err := canonicalToAnth(agent.Message{Role: agent.RoleSystem, Content: "ignored"}, nil); err != nil || msg != nil {
		t.Fatalf("system canonical = %#v err %v", msg, err)
	}
	assistant, err := canonicalToAnth(agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call", Name: "tool"}}}, map[string]string{"tool": "wire_tool"})
	if err != nil {
		t.Fatalf("assistant canonical: %v", err)
	}
	if assistant.Role != "assistant" || assistant.Content[0].Name != "wire_tool" || string(assistant.Content[0].Input) != "{}" {
		t.Fatalf("assistant canonical = %+v", assistant)
	}
	if _, err := canonicalToAnth(agent.Message{Role: agent.RoleTool, Content: "out"}, nil); err == nil || !strings.Contains(err.Error(), "tool_call_id") {
		t.Fatalf("tool without id = %v", err)
	}
	if _, err := canonicalToAnth(agent.Message{Role: "alien", Content: "x"}, nil); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("unknown role = %v", err)
	}
	if th, maxTok := thinkingConfig(1, 1); th == nil || th.BudgetTokens != MinThinkingBudget || maxTok <= th.BudgetTokens {
		t.Fatalf("thinkingConfig = %+v max=%d", th, maxTok)
	}
	if _, err := decodeAnthropicOnBedrockResponse([]byte(`not-json`), "m"); err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("bad anthropic decode = %v", err)
	}
}

func TestBedrockCoverageVendorRoleParamAndDecodeEdges(t *testing.T) {
	if got := ai21JambaRole(agent.RoleAssistant); got != "assistant" {
		t.Fatalf("ai21 assistant role = %q", got)
	}
	if got := ai21JambaRole(agent.RoleTool); got != "user" {
		t.Fatalf("ai21 tool role = %q", got)
	}
	if _, err := encodeAI21JambaOnBedrockRequest("", nil, 10, agent.Params{}, nil); err == nil || !strings.Contains(err.Error(), "at least one message") {
		t.Fatalf("ai21 empty encode = %v", err)
	}
	if _, err := decodeAI21JambaOnBedrockResponse([]byte(`{"choices":[]}`), "ai21.jamba"); err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("ai21 no choices = %v", err)
	}
	if _, err := decodeAI21JambaOnBedrockResponse([]byte(`{"choices":[{"message":{"content":""}}]}`), "ai21.jamba"); err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Fatalf("ai21 empty content = %v", err)
	}

	if got := cohereRole(agent.RoleAssistant); got != "CHATBOT" {
		t.Fatalf("cohere assistant role = %q", got)
	}
	if got := cohereRole(agent.RoleSystem); got != "USER" {
		t.Fatalf("cohere system role = %q", got)
	}
	if _, err := encodeCohereOnBedrockRequest("", []agent.Message{{Role: agent.RoleAssistant, Content: "no user"}}, 10, agent.Params{}, nil); err == nil || !strings.Contains(err.Error(), "user turn") {
		t.Fatalf("cohere no user = %v", err)
	}
	resp, err := decodeCohereOnBedrockResponse([]byte(`{"text":"partial","finish_reason":"MAX_TOKENS"}`), "cohere.command")
	if err != nil {
		t.Fatalf("cohere decode: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens || resp.Message.Content != "partial" {
		t.Fatalf("cohere response = %+v", resp)
	}
	if _, err := decodeCohereOnBedrockResponse([]byte(`{"text":""}`), "cohere.command"); err == nil || !strings.Contains(err.Error(), "empty text") {
		t.Fatalf("cohere empty text = %v", err)
	}

	temp := 0.3
	topP := 0.7
	stop := []string{"END"}
	wire := ai21JambaRequest{}
	wire.applyParams(agent.Params{Temperature: &temp, TopP: &topP, Stop: stop})
	if wire.Temperature != &temp || wire.TopP != &topP || len(wire.Stop) != 1 {
		t.Fatalf("ai21 params = %+v", wire)
	}
	cohereWire := cohereBedrockRequest{}
	cohereWire.applyParams(agent.Params{Temperature: &temp, TopP: &topP, Stop: stop})
	if cohereWire.Temperature != &temp || cohereWire.P != &topP || len(cohereWire.StopSequences) != 1 {
		t.Fatalf("cohere params = %+v", cohereWire)
	}
}

func TestBedrockCoverageCompleteUnsupportedModel(t *testing.T) {
	_, err := New("bearer", "us-east-1").Complete(context.Background(), agent.CompletionRequest{Model: "amazon.titan-text-lite-v1", Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), ErrVendorUnsupported.Error()) {
		t.Fatalf("unsupported model error = %v", err)
	}
}
