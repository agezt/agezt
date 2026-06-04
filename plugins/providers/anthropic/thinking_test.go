// SPDX-License-Identifier: MIT

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_ThinkingEnabled (M318): a positive budget sends an enabled
// thinking block, and max_tokens is bumped to exceed the budget (Anthropic's rule).
func TestEncodeRequest_ThinkingEnabled(t *testing.T) {
	body, err := encodeRequest("claude-sonnet-4-6", "",
		[]agent.Message{{Role: agent.RoleUser, Content: "hard problem"}}, nil, 1000, 4096)
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

// TestEncodeRequest_ThinkingDisabledByDefault: budget 0 omits the block entirely
// (the request wire stays byte-identical for non-thinking runs).
func TestEncodeRequest_ThinkingDisabledByDefault(t *testing.T) {
	body, _ := encodeRequest("m", "", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, 100, 0)
	if strings.Contains(string(body), "thinking") {
		t.Errorf("budget 0 must omit thinking: %s", body)
	}
}

// TestEncodeRequest_ThinkingClampsBudget: a sub-minimum budget is clamped up to
// Anthropic's 1024 floor.
func TestEncodeRequest_ThinkingClampsBudget(t *testing.T) {
	body, _ := encodeRequest("m", "", []agent.Message{{Role: agent.RoleUser, Content: "x"}}, nil, 8000, 500)
	var req struct {
		Thinking *struct {
			BudgetTokens int `json:"budget_tokens"`
		} `json:"thinking"`
	}
	_ = json.Unmarshal(body, &req)
	if req.Thinking == nil || req.Thinking.BudgetTokens != MinThinkingBudget {
		t.Errorf("budget should clamp to %d; got %+v", MinThinkingBudget, req.Thinking)
	}
}

// TestDecodeResponse_CapturesThinking: a thinking content block surfaces on
// ReasoningContent, separate from the answer text.
func TestDecodeResponse_CapturesThinking(t *testing.T) {
	body := []byte(`{"content":[{"type":"thinking","thinking":"Let me reason about this."},{"type":"text","text":"The answer is 42."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":10}}`)
	resp, err := decodeResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningContent != "Let me reason about this." {
		t.Errorf("ReasoningContent=%q", resp.ReasoningContent)
	}
	if resp.Message.Content != "The answer is 42." {
		t.Errorf("Content=%q (thinking must not leak into the answer)", resp.Message.Content)
	}
}

// TestParseStream_Thinking: thinking_delta frames accumulate into ReasoningContent
// and surface as Chunk.ReasoningDelta, separate from the answer's text_delta.
func TestParseStream_Thinking(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"model":"claude","usage":{"input_tokens":5}}}

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
	resp, err := parseStream(strings.NewReader(sse), func(c agent.Chunk) error {
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
