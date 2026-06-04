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

// TestComplete_MistralOnBedrockChatResponse covers the M1.tt
// happy path: a Mistral model id routes through the chat-format
// encoder + decoder; the response surfaces in canonical
// agent.CompletionResponse shape.
func TestComplete_MistralOnBedrockChatResponse(t *testing.T) {
	var captured struct {
		Messages []map[string]any `json:"messages"`
		MaxTok   int              `json:"max_tokens"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "hello from mistral",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	p := bedrock.New("token", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:     "mistral.mistral-large-2407-v1:0",
		System:    "be brief",
		MaxTokens: 256,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hello from mistral" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q want end_turn", resp.StopReason)
	}
	if resp.Usage.Model != "mistral.mistral-large-2407-v1:0" {
		t.Errorf("usage.model = %q", resp.Usage.Model)
	}
	if captured.MaxTok != 256 {
		t.Errorf("max_tokens = %d, want 256", captured.MaxTok)
	}
	// System prompt should be the leading message in the wire body.
	if len(captured.Messages) < 2 {
		t.Fatalf("expected >=2 messages (system + user), got %d", len(captured.Messages))
	}
	if captured.Messages[0]["role"] != "system" {
		t.Errorf("first message role = %v, want system", captured.Messages[0]["role"])
	}
	if captured.Messages[0]["content"] != "be brief" {
		t.Errorf("system content = %v", captured.Messages[0]["content"])
	}
}

// TestComplete_MistralOnBedrock_UsageFromHeaders (M327): Mistral's body carries
// no token counts, so the governor would see zero spend. The Bedrock
// X-Amzn-Bedrock-*-Token-Count response headers must be overlaid onto Usage.
func TestComplete_MistralOnBedrock_UsageFromHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Amzn-Bedrock-Input-Token-Count", "37")
		w.Header().Set("X-Amzn-Bedrock-Output-Token-Count", "14")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mistral.mistral-large-2407-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Usage.InputTokens != 37 || resp.Usage.OutputTokens != 14 {
		t.Errorf("usage = %d/%d, want 37/14 from headers", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	if resp.Usage.Model != "mistral.mistral-large-2407-v1:0" {
		t.Errorf("usage.model = %q", resp.Usage.Model)
	}
}

// TestComplete_InlineUsageNotOverriddenByHeaders (M327): a vendor with inline
// body usage (Nova) must keep its body-derived counts even when the headers
// disagree — the body is richer (and the header overlay only fills zeros).
func TestComplete_InlineUsageNotOverriddenByHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Amzn-Bedrock-Input-Token-Count", "999")
		w.Header().Set("X-Amzn-Bedrock-Output-Token-Count", "999")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":     map[string]any{"message": map[string]any{"role": "assistant", "content": []map[string]any{{"text": "hi"}}}},
			"stopReason": "end_turn",
			"usage":      map[string]any{"inputTokens": 5, "outputTokens": 3},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "amazon.nova-pro-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %d/%d, want inline 5/3 (headers must not override inline)", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
}

// TestComplete_MistralOnBedrock_LengthStop verifies finish_reason
// "length" maps to canonical StopMaxTokens.
func TestComplete_MistralOnBedrock_LengthStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "trunc..."},
				"finish_reason": "length",
			}},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mistral.mistral-large-2407-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q want max_tokens", resp.StopReason)
	}
}

// TestComplete_RegionalMistralAccepted verifies cross-inference
// profile ids (`eu.mistral.*`) route through the Mistral path.
func TestComplete_RegionalMistralAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "eu.mistral.mistral-large-2407-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Errorf("Complete: %v (regional Mistral profile should be accepted)", err)
	}
}

// TestComplete_MistralEmptyChoicesErrors: defensive — server
// returning empty choices is treated as an error rather than
// silently returning an empty message.
func TestComplete_MistralEmptyChoicesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mistral.mistral-7b-instruct-v0:2",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on empty choices, got nil")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error doesn't mention empty choices: %v", err)
	}
}
