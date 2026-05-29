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

// TestComplete_MetaLlama_HappyPath verifies the M1.tt-3 path:
// canonical messages render to the Llama 3 prompt template,
// response generation surfaces in canonical shape, token counts
// flow into Usage.
func TestComplete_MetaLlama_HappyPath(t *testing.T) {
	var capturedPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Prompt    string `json:"prompt"`
			MaxGenLen int    `json:"max_gen_len"`
		}
		_ = json.Unmarshal(body, &req)
		capturedPrompt = req.Prompt
		_ = json.NewEncoder(w).Encode(map[string]any{
			"generation":             "llama response",
			"stop_reason":            "stop",
			"prompt_token_count":     42,
			"generation_token_count": 7,
		})
	}))
	defer srv.Close()

	p := bedrock.New("token", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:     "meta.llama3-1-70b-instruct-v1:0",
		System:    "be terse",
		MaxTokens: 256,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "llama response" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 42 {
		t.Errorf("InputTokens = %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 7 {
		t.Errorf("OutputTokens = %d", resp.Usage.OutputTokens)
	}
	// Prompt must include the system + user turns wrapped in the
	// Llama 3 template tokens.
	for _, want := range []string{
		"<|begin_of_text|>",
		"<|start_header_id|>system<|end_header_id|>",
		"be terse",
		"<|start_header_id|>user<|end_header_id|>",
		"hi",
		"<|eot_id|>",
		// Trailing open assistant header so the model continues:
		"<|start_header_id|>assistant<|end_header_id|>",
	} {
		if !strings.Contains(capturedPrompt, want) {
			t.Errorf("prompt missing %q: %q", want, capturedPrompt)
		}
	}
}

// TestComplete_MetaLlama_LengthStop verifies stop_reason "length"
// maps to StopMaxTokens.
func TestComplete_MetaLlama_LengthStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"generation":  "...",
			"stop_reason": "length",
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "meta.llama3-8b-instruct-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q want max_tokens", resp.StopReason)
	}
}
