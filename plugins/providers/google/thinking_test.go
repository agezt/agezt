// SPDX-License-Identifier: MIT

package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_ThinkingEnabled (M319): a non-zero budget sends a
// thinkingConfig with includeThoughts:true so Gemini returns the thought
// summaries, and the budget is carried verbatim.
func TestEncodeRequest_ThinkingEnabled(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hard problem"}}
	body, err := encodeRequest("", msgs, nil, 2048, false, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		GenerationConfig struct {
			ThinkingConfig *struct {
				IncludeThoughts bool `json:"includeThoughts"`
				ThinkingBudget  int  `json:"thinkingBudget"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	tc := req.GenerationConfig.ThinkingConfig
	if tc == nil {
		t.Fatalf("thinkingConfig missing: %s", body)
	}
	if !tc.IncludeThoughts {
		t.Errorf("includeThoughts must be true so summaries return: %s", body)
	}
	if tc.ThinkingBudget != 1024 {
		t.Errorf("thinkingBudget=%d, want 1024", tc.ThinkingBudget)
	}
}

// TestEncodeRequest_ThinkingDynamicBudget: -1 (dynamic) is a legitimate,
// distinct opt-in and must be sent (not treated as "off").
func TestEncodeRequest_ThinkingDynamicBudget(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "x"}}
	body, _ := encodeRequest("", msgs, nil, 0, false, -1)
	var req struct {
		GenerationConfig *struct {
			ThinkingConfig *struct {
				ThinkingBudget int `json:"thinkingBudget"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	_ = json.Unmarshal(body, &req)
	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
		t.Fatalf("dynamic budget (-1) must still send thinkingConfig: %s", body)
	}
	if got := req.GenerationConfig.ThinkingConfig.ThinkingBudget; got != -1 {
		t.Errorf("thinkingBudget=%d, want -1", got)
	}
}

// TestEncodeRequest_ThinkingDisabledByDefault: budget 0 omits thinkingConfig
// entirely — the request wire is byte-identical to a non-thinking run.
func TestEncodeRequest_ThinkingDisabledByDefault(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi"}}
	body, _ := encodeRequest("", msgs, nil, 100, false, 0)
	if strings.Contains(string(body), "thinkingConfig") {
		t.Errorf("budget 0 must omit thinkingConfig: %s", body)
	}
}

// TestDecodeResponse_CapturesThought: a part flagged thought:true surfaces on
// ReasoningContent (not the answer), and thoughtsTokenCount folds into the
// billable OutputTokens.
func TestDecodeResponse_CapturesThought(t *testing.T) {
	body := []byte(`{"candidates":[{"content":{"role":"model","parts":[` +
		`{"text":"Let me think.","thought":true},` +
		`{"text":"The answer is 42."}` +
		`]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10,"thoughtsTokenCount":7}}`)
	resp, err := decodeResponse(body, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReasoningContent != "Let me think." {
		t.Errorf("ReasoningContent=%q", resp.ReasoningContent)
	}
	if resp.Message.Content != "The answer is 42." {
		t.Errorf("Content=%q (thought must not leak into the answer)", resp.Message.Content)
	}
	// 10 candidates + 7 thoughts = 17 billable output tokens.
	if resp.Usage.OutputTokens != 17 {
		t.Errorf("OutputTokens=%d, want 17 (candidates+thoughts)", resp.Usage.OutputTokens)
	}
}

// TestParseStream_Thought: thought-flagged text deltas accumulate into
// ReasoningContent and surface as Chunk.ReasoningDelta, separate from the
// answer's plain text deltas. thoughtsTokenCount folds into OutputTokens.
func TestParseStream_Thought(t *testing.T) {
	sse := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hmm, ","thought":true}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":"42.","thought":true}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":"The answer"}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":" is 42."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":4,"thoughtsTokenCount":6}}

`
	var reasoning, text strings.Builder
	resp, err := parseStream(strings.NewReader(sse), "gemini-2.5-flash", func(c agent.Chunk) error {
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
	if resp.Message.Content != "The answer is 42." {
		t.Errorf("resp.Content=%q (thought must not leak into the answer)", resp.Message.Content)
	}
	if resp.Usage.OutputTokens != 10 {
		t.Errorf("OutputTokens=%d, want 10 (candidates 4 + thoughts 6)", resp.Usage.OutputTokens)
	}
}
