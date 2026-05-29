// SPDX-License-Identifier: MIT

package bedrock_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/bedrock"
)

// TestComplete_CohereOnBedrock_HappyPath verifies the M1.tt-2
// Cohere chat shape: latest user → message field, earlier turns
// → chat_history, system → preamble.
func TestComplete_CohereOnBedrock_HappyPath(t *testing.T) {
	var captured struct {
		Message     string           `json:"message"`
		ChatHistory []map[string]any `json:"chat_history"`
		Preamble    string           `json:"preamble"`
		MaxTokens   int              `json:"max_tokens"`
		Extra       map[string]any   `json:"-"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":          "hello from cohere",
			"finish_reason": "COMPLETE",
			"generation_id": "gen-123",
		})
	}))
	defer srv.Close()

	p := bedrock.New("token", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:     "cohere.command-r-plus-v1:0",
		System:    "be friendly",
		MaxTokens: 512,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "what's up?"},
			{Role: agent.RoleAssistant, Content: "not much, you?"},
			{Role: agent.RoleUser, Content: "want to chat"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hello from cohere" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.Message.Role != agent.RoleAssistant {
		t.Errorf("role = %q", resp.Message.Role)
	}
	if captured.Message != "want to chat" {
		t.Errorf("message field = %q, want latest user turn", captured.Message)
	}
	if captured.Preamble != "be friendly" {
		t.Errorf("preamble = %q, want %q", captured.Preamble, "be friendly")
	}
	if captured.MaxTokens != 512 {
		t.Errorf("max_tokens = %d", captured.MaxTokens)
	}
	if len(captured.ChatHistory) != 2 {
		t.Fatalf("chat_history len = %d, want 2 (prior user + assistant)", len(captured.ChatHistory))
	}
	if captured.ChatHistory[0]["role"] != "USER" {
		t.Errorf("history[0].role = %v, want USER", captured.ChatHistory[0]["role"])
	}
	if captured.ChatHistory[1]["role"] != "CHATBOT" {
		t.Errorf("history[1].role = %v, want CHATBOT", captured.ChatHistory[1]["role"])
	}
}

// TestComplete_CohereOnBedrock_MaxTokensStop verifies MAX_TOKENS
// finish_reason maps to canonical StopMaxTokens.
func TestComplete_CohereOnBedrock_MaxTokensStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":          "trunc",
			"finish_reason": "MAX_TOKENS",
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "cohere.command-r-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q want max_tokens", resp.StopReason)
	}
}

// TestComplete_CohereOnBedrock_NoUserTurnErrors: malformed
// invocation with only assistant messages should error rather
// than silently sending an empty message to Cohere.
func TestComplete_CohereOnBedrock_NoUserTurnErrors(t *testing.T) {
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = "http://localhost:1"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "cohere.command-r-v1:0",
		Messages: []agent.Message{{Role: agent.RoleAssistant, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for assistant-only messages")
	}
}
