// SPDX-License-Identifier: MIT

package bedrock_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/bedrock"
)

// TestComplete_DeepSeekOnBedrock covers the M328 happy path: a deepseek.r1-* id
// renders the DeepSeek chat-template prompt (with the trailing <think>), and the
// completion text is split into ReasoningContent (before </think>) and the answer
// (after). Token usage comes from the Bedrock response headers (M327).
func TestComplete_DeepSeekOnBedrock(t *testing.T) {
	var captured struct {
		Prompt    string `json:"prompt"`
		MaxTokens int    `json:"max_tokens"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("X-Amzn-Bedrock-Input-Token-Count", "20")
		w.Header().Set("X-Amzn-Bedrock-Output-Token-Count", "30")
		// The prompt opened <think>, so the text is reasoning then </think> then answer.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"text":        "6 times 7 is 42.</think>\n\nThe answer is 42.",
				"stop_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	p := bedrock.New("token", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:     "deepseek.r1-v1:0",
		System:    "be precise",
		MaxTokens: 512,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: "what is 6*7?"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "The answer is 42." {
		t.Errorf("answer = %q (reasoning must be stripped)", resp.Message.Content)
	}
	if resp.ReasoningContent != "6 times 7 is 42." {
		t.Errorf("reasoning = %q", resp.ReasoningContent)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 30 {
		t.Errorf("usage = %d/%d, want 20/30 from headers", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	// Wire: the DeepSeek chat template with system prefix, user turn, and trailing
	// assistant + open-think tokens.
	for _, want := range []string{
		"<｜begin▁of▁sentence｜>",
		"be precise",
		"<｜User｜>what is 6*7?",
		"<｜Assistant｜><think>\n",
	} {
		if !strings.Contains(captured.Prompt, want) {
			t.Errorf("prompt missing %q in:\n%q", want, captured.Prompt)
		}
	}
	if captured.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want 512", captured.MaxTokens)
	}
}

// TestComplete_DeepSeekRegionalAccepted verifies cross-inference profile ids
// (`us.deepseek.r1-v1:0`) route through the DeepSeek path.
func TestComplete_DeepSeekRegionalAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"text": "thinking</think>ok", "stop_reason": "stop"}},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "us.deepseek.r1-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Errorf("Complete: %v (regional DeepSeek profile should be accepted)", err)
	}
}

// TestComplete_DeepSeekTruncatedThinking: when the response has no </think>
// (truncated mid-thought at max_tokens), the text surfaces as the answer rather
// than vanishing into reasoning — the caller never gets an empty response.
func TestComplete_DeepSeekTruncatedThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"text": "still reasoning, never finished", "stop_reason": "length"}},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "deepseek.r1-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "still reasoning, never finished" {
		t.Errorf("answer = %q (no </think> → text is the answer)", resp.Message.Content)
	}
	if resp.ReasoningContent != "" {
		t.Errorf("reasoning = %q, want empty when no close tag", resp.ReasoningContent)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q want max_tokens", resp.StopReason)
	}
}

// TestComplete_DeepSeekEmptyChoicesErrors: defensive — empty choices is an error.
func TestComplete_DeepSeekEmptyChoicesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "deepseek.r1-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on empty choices, got nil")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error doesn't mention empty choices: %v", err)
	}
}
