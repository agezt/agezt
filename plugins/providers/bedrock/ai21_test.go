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

// TestComplete_AI21Jamba_HappyPath verifies the M1.tt-4 path:
// canonical messages render to the OpenAI-style chat completion
// shape, response content surfaces in canonical shape, token
// counts flow into Usage.
func TestComplete_AI21Jamba_HappyPath(t *testing.T) {
	var bodyOnWire map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &bodyOnWire)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chat-1",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "jamba response",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 5,
				"total_tokens":      16,
			},
		})
	}))
	defer srv.Close()

	p := bedrock.New("token", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:     "ai21.jamba-1-5-large-v1:0",
		System:    "be terse",
		MaxTokens: 128,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "jamba response" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}

	// System message must lead the messages array (Jamba/OpenAI
	// convention — system inline, not a separate field like
	// Anthropic). User turn follows.
	msgs, ok := bodyOnWire["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages on wire = %v", bodyOnWire["messages"])
	}
	sysMsg, _ := msgs[0].(map[string]any)
	if sysMsg["role"] != "system" || sysMsg["content"] != "be terse" {
		t.Errorf("system message = %v", sysMsg)
	}
	userMsg, _ := msgs[1].(map[string]any)
	if userMsg["role"] != "user" || userMsg["content"] != "hi" {
		t.Errorf("user message = %v", userMsg)
	}
}

// TestComplete_AI21Jamba_LengthStop verifies finish_reason "length"
// maps to StopMaxTokens (matches the convention from the other
// chat-shape vendors).
func TestComplete_AI21Jamba_LengthStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "..."},
				"finish_reason": "length",
			}},
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "ai21.jamba-instruct-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q want max_tokens", resp.StopReason)
	}
}

// TestComplete_AI21Jamba_RegionalProfile verifies the cross-region
// inference profile id pattern (`us.ai21.jamba-...`) routes to the
// AI21 body shape, matching the regional-profile recognition the
// other vendors already do.
func TestComplete_AI21Jamba_RegionalProfile(t *testing.T) {
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
		Model:    "us.ai21.jamba-1-5-large-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("regional Jamba profile should route: %v", err)
	}
}

// TestComplete_AI21LegacyJ2Refused: AI21's older J2 SKU (jurassic-2)
// uses a different request shape and is intentionally NOT wired.
// It must hit ErrVendorUnsupported so the operator gets a clear
// message rather than a confusing 400 from Bedrock.
func TestComplete_AI21LegacyJ2Refused(t *testing.T) {
	p := bedrock.New("k", "us-east-1")
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "ai21.j2-ultra-v1",
	})
	if err == nil {
		t.Fatal("expected error for ai21.j2-* (legacy SKU)")
	}
	// J2 isn't matched by isAI21JambaModel so it falls through to
	// the default ErrVendorUnsupported branch — the exact error
	// content is checked via the existing TestComplete_UnsupportedVendorRefused
	// pattern; here we just confirm it errors.
}
