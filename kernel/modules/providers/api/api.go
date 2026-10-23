// SPDX-License-Identifier: MIT

// Package api is the providers module's gate: the vocabulary the module owns
// and the seams it needs from the world above it.
//
// The module decides WHICH provider runs and HOW it is registered. The wire
// adapters that actually speak to a provider — the compat family, the offline
// mock, the ChatGPT Responses backend — live above the module, in the plugin
// layer, and the middleware stack lives in the runs module. A module may not
// import a plugin, so the engine asks for those adapters through the Ports
// interface below and the composition root (cmd/agezt) fills them in.
//
// The model row is the module's own: the module owns what a model IS (which
// entries reach an operator-facing picker, how they are ordered), and the
// adapters map their wire shapes into it.
package api

import (
	"context"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
)

// ModelInfo is the module's own model row — the subset of a discovered model
// entry the module routes and describes with. The JSON tags match the Codex
// CLI's models_cache.json, which is the same reply the backend serves, so the
// offline cache path and the backend path speak one shape.
type ModelInfo struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// Visibility is "list" for models meant for the picker and "hide" for
	// internal ones (weight-matched variants, the auto-review model).
	Visibility string `json:"visibility"`
	// Priority is the backend's own ordering — lower is more preferred, and the
	// lowest listed entry is what an operator gets by default.
	Priority         int      `json:"priority"`
	ContextWindow    int      `json:"context_window"`
	MaxContextWindow int      `json:"max_context_window"`
	DefaultReasoning string   `json:"default_reasoning_level"`
	InputModalities  []string `json:"input_modalities"`
	SupportedInAPI   bool     `json:"supported_in_api"`
	ParallelTools    bool     `json:"supports_parallel_tool_calls"`
	// BaseInstructions is this model's own system prompt. The backend now serves
	// one per model; sending a single vendored prompt to a model that expects
	// its own is how newer models start misbehaving.
	BaseInstructions string `json:"base_instructions"`
}

// Listed reports whether the model belongs in an operator-facing picker.
func (m ModelInfo) Listed() bool {
	return strings.TrimSpace(m.Slug) != "" && !strings.EqualFold(strings.TrimSpace(m.Visibility), "hide")
}

// SortModels drops entries without a slug and orders them by the backend's own
// priority (most preferred first), the order both the backend reply and the
// Codex CLI cache are stored in. The module owns this rule, so the two
// discovery paths cannot drift apart.
func SortModels(in []ModelInfo) []ModelInfo {
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

// TokenSource is the module's own seam on the ChatGPT OAuth token store. The
// module owns the token lifecycle (it holds the vault); the adapter above it
// only needs to read a token, so it depends on this shape and not on the
// store's concrete type.
type TokenSource interface {
	// HasTokens reports whether a usable token set is stored.
	HasTokens() bool
	// Token returns a valid access token + account id, refreshing proactively
	// when the access token is within the refresh skew of expiry.
	Token(ctx context.Context) (access, accountID string, err error)
	// ForceRefresh refreshes regardless of expiry (used on a 401).
	ForceRefresh(ctx context.Context) (access, accountID string, err error)
	// Account returns the connected account's email + id (best-effort).
	Account() (email, accountID string)
}

// Ports are the seams the providers module needs from the adapters above it.
// The composition root fills them in; the module never reaches up for them.
//
// It is an interface on purpose: a missing adapter is a compile error at the
// wiring site, not a nil dereference on a boot path.
type Ports interface {
	// Build constructs a wire provider for a catalog entry. It returns the
	// provider, the model id the wire was constructed with, and an error.
	Build(entry *catalog.Provider, modelID string, lookup func(string) string) (llm.Provider, string, error)
	// SupportedFamily reports whether the wire adapter can serve a catalog
	// family at all.
	SupportedFamily(family catalog.Family) bool
	// FirstModelID picks the model a wire is constructed with when the
	// operator pinned none — the inert construction placeholder.
	FirstModelID(entry *catalog.Provider) string
	// DemoEcho builds the explicit offline demo provider (the
	// AGEZT_DEMO_ECHO=1 escape hatch). The daemon never picks it on its own.
	DemoEcho() llm.Provider
	// Wrap returns the wrapper that applies the operator's middleware stack to
	// a wire provider. The engine asks for it once per boot cycle, so the
	// stack is built once and applied to the primary and every alternate
	// alike — a reload must not silently register unwrapped providers.
	Wrap(get func(string) string) func(llm.Provider) llm.Provider
	// ChatGPTDiscover lists the ChatGPT model surface from the backend.
	ChatGPTDiscover(src TokenSource) ([]ModelInfo, bool)
	// ChatGPTProvider builds the ChatGPT wire provider for a model, with the
	// per-model system prompts attached.
	ChatGPTProvider(src TokenSource, model string, instructions map[string]string) llm.Provider
	// ChatGPTBaseURL is the backend base URL recorded in the catalog entry.
	ChatGPTBaseURL() string
}
