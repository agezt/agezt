// SPDX-License-Identifier: MIT

package openairesponses

// Model discovery against the ChatGPT backend's /models endpoint — the same call
// Codex CLI makes to fill its model picker (it caches the reply in
// ~/.codex/models_cache.json). Without discovery the served model set is a
// hand-written constant that goes stale the moment OpenAI ships a new Codex
// model, leaving operators routing to model ids the backend no longer knows.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/plugins/providers/internal/retry"
)

// ClientVersion is the Codex CLI version we present to the backend. /models
// REQUIRES it as a query parameter (omitting it is a 400) and gates each entry
// on that model's own minimal_client_version — so a version older than a new
// model's floor silently hides it from the reply.
const ClientVersion = "0.146.0"

// modelsTimeout bounds a discovery call. Discovery sits on the boot/reload path,
// so it must fail fast rather than hold the daemon.
const modelsTimeout = 20 * time.Second

// ModelInfo is the subset of a /models entry AGEZT uses. The backend sends far
// more (tool modes, service tiers, truncation policy); everything not needed to
// route or describe a model is deliberately dropped.
type ModelInfo struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// Visibility is "list" for models meant for the picker and "hide" for
	// internal ones (weight-matched variants, the auto-review model).
	Visibility string `json:"visibility"`
	// Priority is the backend's own ordering — lower is more preferred, and the
	// lowest listed entry is what Codex CLI defaults to.
	Priority         int      `json:"priority"`
	ContextWindow    int      `json:"context_window"`
	MaxContextWindow int      `json:"max_context_window"`
	DefaultReasoning string   `json:"default_reasoning_level"`
	InputModalities  []string `json:"input_modalities"`
	SupportedInAPI   bool     `json:"supported_in_api"`
	ParallelTools    bool     `json:"supports_parallel_tool_calls"`
	// BaseInstructions is this model's own system prompt. The backend now serves
	// one per model; sending the single vendored instructions.md to a model that
	// expects its own is how newer models start misbehaving.
	BaseInstructions string `json:"base_instructions"`
}

// Listed reports whether the model belongs in an operator-facing picker.
func (m ModelInfo) Listed() bool {
	return m.Slug != "" && !strings.EqualFold(strings.TrimSpace(m.Visibility), "hide")
}

// ListModels fetches the models the signed-in account may use, ordered by the
// backend's own priority (most-preferred first). base defaults to
// DefaultBaseURL. Hidden entries are returned too — callers filter with Listed.
func ListModels(ctx context.Context, base string, token TokenFunc) ([]ModelInfo, error) {
	if token == nil {
		return nil, fmt.Errorf("openairesponses: no token source")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	endpoint := base + "/models?client_version=" + url.QueryEscape(ClientVersion)

	ctx, cancel := context.WithTimeout(ctx, modelsTimeout)
	defer cancel()

	raw, status, err := getModels(ctx, endpoint, token, false)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		// Reactive refresh-and-retry once, mirroring Complete.
		if raw, status, err = getModels(ctx, endpoint, token, true); err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 600 {
			msg = msg[:600]
		}
		return nil, fmt.Errorf("openairesponses: /models status %d: %s", status, msg)
	}

	var payload struct {
		Models []ModelInfo `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("openairesponses: parse /models: %w", err)
	}
	models := ParseModels(payload.Models)
	if len(models) == 0 {
		return nil, fmt.Errorf("openairesponses: /models returned no usable models")
	}
	return models, nil
}

// ParseModels drops entries without a slug and sorts by backend priority. Shared
// with the Codex CLI cache reader so both sources yield the same ordering.
func ParseModels(in []ModelInfo) []ModelInfo {
	out := make([]ModelInfo, 0, len(in))
	for _, m := range in {
		if strings.TrimSpace(m.Slug) != "" {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// getModels performs one GET, returning the body + status. When force, it asks
// the token source to refresh first (first attempt only — later retry rounds
// reuse the fresh token instead of hammering the OAuth endpoint).
func getModels(ctx context.Context, endpoint string, token TokenFunc, force bool) ([]byte, int, error) {
	first := true
	raw, _, err := retry.DoHTTP(ctx, httpClientFor(modelsTimeout), func() (*http.Request, error) {
		access, accountID, err := token(ctx, force && first)
		first = false
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+access)
		if accountID != "" {
			req.Header.Set("chatgpt-account-id", accountID)
		}
		req.Header.Set("originator", originator)
		req.Header.Set("version", ClientVersion)
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, 8<<20)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return []byte(h.Body), h.StatusCode, nil
		}
		return nil, 0, err
	}
	return raw, http.StatusOK, nil
}
