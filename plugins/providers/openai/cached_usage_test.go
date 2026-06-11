// SPDX-License-Identifier: MIT

package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/openai"
)

// TestComplete_CachedTokens_BothSpellings (M887): both wire spellings of
// "prompt tokens served from cache" land on agent.Usage.CachedInputTokens —
// OpenAI's prompt_tokens_details.cached_tokens and DeepSeek's top-level
// prompt_cache_hit_tokens. Without the DeepSeek fallback, a DeepSeek run's
// cache hits were silently billed as fresh input.
func TestComplete_CachedTokens_BothSpellings(t *testing.T) {
	cases := []struct {
		name  string
		usage map[string]any
		want  int
	}{
		{
			name: "openai prompt_tokens_details",
			usage: map[string]any{
				"prompt_tokens": 1000, "completion_tokens": 10,
				"prompt_tokens_details": map[string]any{"cached_tokens": 800},
			},
			want: 800,
		},
		{
			name: "deepseek prompt_cache_hit_tokens",
			usage: map[string]any{
				"prompt_tokens": 1000, "completion_tokens": 10,
				"prompt_cache_hit_tokens": 700, "prompt_cache_miss_tokens": 300,
			},
			want: 700,
		},
		{
			name: "both spellings (no double count)",
			usage: map[string]any{
				"prompt_tokens": 1000, "completion_tokens": 10,
				"prompt_tokens_details":   map[string]any{"cached_tokens": 600},
				"prompt_cache_hit_tokens": 600,
			},
			want: 600,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "cmpl-c", "object": "chat.completion", "model": "deepseek-chat",
					"choices": []map[string]any{{
						"index":         0,
						"message":       map[string]any{"role": "assistant", "content": "ok"},
						"finish_reason": "stop",
					}},
					"usage": c.usage,
				})
			}))
			defer srv.Close()

			p := openai.New("sk-test")
			p.Endpoint = srv.URL
			resp, err := p.Complete(context.Background(), agent.CompletionRequest{
				Model:    "deepseek-chat",
				Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
			})
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if resp.Usage.CachedInputTokens != c.want {
				t.Errorf("CachedInputTokens = %d, want %d", resp.Usage.CachedInputTokens, c.want)
			}
			if resp.Usage.InputTokens != 1000 {
				t.Errorf("InputTokens = %d, want 1000 (cache hits stay a subset, not a deduction)", resp.Usage.InputTokens)
			}
		})
	}
}
