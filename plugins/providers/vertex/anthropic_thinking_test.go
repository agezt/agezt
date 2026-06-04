// SPDX-License-Identifier: MIT

package vertex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeAnthropicOnVertex_ThinkingEnabled (M321): a positive budget sends an
// enabled thinking block and bumps max_tokens above the budget (Anthropic's rule).
func TestEncodeAnthropicOnVertex_ThinkingEnabled(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hard problem"}}
	body, err := encodeAnthropicOnVertexRequest("", msgs, nil, 1000, 4096, false)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		MaxTokens int `json:"max_tokens"`
		Thinking  *struct {
			Type         string `json:"type"`
			BudgetTokens int    `json:"budget_tokens"`
		} `json:"thinking"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.Thinking == nil || req.Thinking.Type != "enabled" || req.Thinking.BudgetTokens != 4096 {
		t.Fatalf("thinking block wrong: %s", body)
	}
	if req.MaxTokens <= req.Thinking.BudgetTokens {
		t.Errorf("max_tokens %d must exceed thinking budget %d", req.MaxTokens, req.Thinking.BudgetTokens)
	}
}

// TestEncodeAnthropicOnVertex_ThinkingDisabledByDefault: budget 0 (and negative,
// the Gemini-only "dynamic" value) omit the block — wire byte-identical.
func TestEncodeAnthropicOnVertex_ThinkingDisabledByDefault(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	for _, budget := range []int{0, -1} {
		body, _ := encodeAnthropicOnVertexRequest("", msgs, nil, 100, budget, false)
		if strings.Contains(string(body), "thinking") {
			t.Errorf("budget %d must omit thinking: %s", budget, body)
		}
	}
}

// TestEncodeAnthropicOnVertex_ThinkingClampsBudget: a sub-1024 budget is clamped
// up to Anthropic's floor.
func TestEncodeAnthropicOnVertex_ThinkingClampsBudget(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "x"}}
	body, _ := encodeAnthropicOnVertexRequest("", msgs, nil, 8000, 500, false)
	var req struct {
		Thinking *struct {
			BudgetTokens int `json:"budget_tokens"`
		} `json:"thinking"`
	}
	_ = json.Unmarshal(body, &req)
	if req.Thinking == nil || req.Thinking.BudgetTokens != MinAnthropicThinkingBudget {
		t.Errorf("budget should clamp to %d; got %+v", MinAnthropicThinkingBudget, req.Thinking)
	}
}

// TestDecodeAnthropicOnVertex_CapturesThinking: a thinking content block surfaces
// on ReasoningContent, separate from the answer text.
func TestDecodeAnthropicOnVertex_CapturesThinking(t *testing.T) {
	body := []byte(`{"content":[{"type":"thinking","thinking":"Let me reason."},{"type":"text","text":"The answer is 42."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":10}}`)
	resp, err := decodeAnthropicOnVertexResponse(body, "claude-opus-4-7@20251031")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningContent != "Let me reason." {
		t.Errorf("ReasoningContent=%q", resp.ReasoningContent)
	}
	if resp.Message.Content != "The answer is 42." {
		t.Errorf("Content=%q (thinking must not leak into the answer)", resp.Message.Content)
	}
}

// TestParseAnthropicSSE_Thinking: thinking_delta frames accumulate into
// ReasoningContent and surface as Chunk.ReasoningDelta, separate from text_delta.
func TestParseAnthropicSSE_Thinking(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Hmm, "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"42."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"42"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}

event: message_stop
data: {"type":"message_stop"}

`
	var reasoning, text strings.Builder
	resp, err := parseAnthropicSSE(strings.NewReader(sse), "claude-opus-4-7@20251031", func(c agent.Chunk) error {
		reasoning.WriteString(c.ReasoningDelta)
		text.WriteString(c.TextDelta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reasoning.String() != "Hmm, 42." {
		t.Errorf("streamed reasoning=%q", reasoning.String())
	}
	if resp.ReasoningContent != "Hmm, 42." {
		t.Errorf("resp.ReasoningContent=%q", resp.ReasoningContent)
	}
	if resp.Message.Content != "42" {
		t.Errorf("resp.Content=%q (thinking must not leak into the answer)", resp.Message.Content)
	}
}
