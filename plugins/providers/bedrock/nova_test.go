// SPDX-License-Identifier: MIT

package bedrock_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/bedrock"
)

// TestComplete_NovaOnBedrock covers the M326 happy path: an amazon.nova-* id
// routes through the "messages-v1" encoder/decoder; the system prompt lands in
// the top-level `system` array, user content as a content-block array, and the
// output text + inline usage surface in canonical shape.
func TestComplete_NovaOnBedrock(t *testing.T) {
	var captured struct {
		SchemaVersion string `json:"schemaVersion"`
		System        []struct {
			Text string `json:"text"`
		} `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		InferenceConfig struct {
			MaxTokens int `json:"maxTokens"`
		} `json:"inferenceConfig"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"text": "hello from nova"}},
			}},
			"stopReason": "end_turn",
			"usage":      map[string]any{"inputTokens": 12, "outputTokens": 7, "totalTokens": 19},
		})
	}))
	defer srv.Close()

	p := bedrock.New("token", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:     "amazon.nova-pro-v1:0",
		System:    "be brief",
		MaxTokens: 256,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hello from nova" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 7 {
		t.Errorf("usage tokens = %d/%d, want 12/7", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	if resp.Usage.Model != "amazon.nova-pro-v1:0" {
		t.Errorf("usage.model = %q", resp.Usage.Model)
	}
	// Wire shape assertions.
	if captured.SchemaVersion != "messages-v1" {
		t.Errorf("schemaVersion = %q, want messages-v1", captured.SchemaVersion)
	}
	if captured.InferenceConfig.MaxTokens != 256 {
		t.Errorf("maxTokens = %d, want 256", captured.InferenceConfig.MaxTokens)
	}
	if len(captured.System) != 1 || captured.System[0].Text != "be brief" {
		t.Errorf("system = %+v, want one block 'be brief'", captured.System)
	}
	if len(captured.Messages) != 1 || captured.Messages[0].Role != "user" ||
		len(captured.Messages[0].Content) != 1 || captured.Messages[0].Content[0].Text != "hi" {
		t.Errorf("messages = %+v", captured.Messages)
	}
}

// TestComplete_NovaOnBedrock_MaxTokensStop verifies stopReason "max_tokens"
// maps to canonical StopMaxTokens.
func TestComplete_NovaOnBedrock_MaxTokensStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":     map[string]any{"message": map[string]any{"role": "assistant", "content": []map[string]any{{"text": "trunc..."}}}},
			"stopReason": "max_tokens",
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "amazon.nova-lite-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q want max_tokens", resp.StopReason)
	}
}

// TestComplete_RegionalNovaAccepted verifies cross-inference profile ids
// (`us.amazon.nova-*`) route through the Nova path.
func TestComplete_RegionalNovaAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":     map[string]any{"message": map[string]any{"role": "assistant", "content": []map[string]any{{"text": "ok"}}}},
			"stopReason": "end_turn",
		})
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "us.amazon.nova-premier-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Errorf("Complete: %v (regional Nova profile should be accepted)", err)
	}
}

// TestComplete_TitanStaysUnwired verifies the legacy amazon.titan-* family is
// NOT swept up by Nova detection — it still returns ErrVendorUnsupported.
func TestComplete_TitanStaysUnwired(t *testing.T) {
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = "http://127.0.0.1:0" // never reached; dispatch fails first
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "amazon.titan-text-express-v1",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if !errors.Is(err, bedrock.ErrVendorUnsupported) {
		t.Errorf("titan should stay unsupported, got err=%v", err)
	}
}

// TestComplete_NovaEmptyOutputErrors: defensive — a response with no output text
// is an error, not a silent empty message.
func TestComplete_NovaEmptyOutputErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[]}},"stopReason":"end_turn"}`))
	}))
	defer srv.Close()
	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "amazon.nova-micro-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on empty output, got nil")
	}
	if !strings.Contains(err.Error(), "no output text") {
		t.Errorf("error doesn't mention empty output: %v", err)
	}
}
