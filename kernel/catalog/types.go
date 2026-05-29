// SPDX-License-Identifier: MIT

// Package catalog is the live provider/model registry. It replaces the
// hardcoded `modelPriceTable` in kernel/governor/pricing.go and the
// per-provider Go packages in plugins/providers/* with a single
// data-driven source of truth (TASKS P1-CONDUIT-04, SPEC-15 §1).
//
// Schema mirrors models.dev/api.json (the community catalog the
// project syncs from): a flat map of provider-id → Provider, where
// each Provider carries its base URL, credential env-var name, npm/SDK
// hint (which the Governor uses to pick the right wire dialect), and
// a map of model-id → Model with prices in USD per-million-tokens.
//
// On disk under <BaseDir>/catalog/:
//
//	api.json        the most-recent remote-synced catalog
//	local.json      provider entries auto-discovered from running
//	                services (Ollama /api/tags, lm-studio, etc.)
//	custom.json     operator-curated overrides; wins over both above
//
// Read precedence: custom > local > api. Writes only ever touch one
// file; the loader merges. This means `agt catalog sync` can refresh
// `api.json` without clobbering local discoveries or hand-edits.
package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Provider is one external service that can serve completions. Field
// names match models.dev/api.json so the JSON unmarshals directly.
type Provider struct {
	// ID is the stable lookup key, e.g. "anthropic", "openai", "ollama".
	ID string `json:"id"`
	// Name is the human label.
	Name string `json:"name"`
	// Env is the list of environment variable names that must be set
	// for this provider's credentials. Multiple entries mean any of
	// them is sufficient (e.g. ["ANTHROPIC_API_KEY", "CLAUDE_API_KEY"]).
	// Empty means the provider needs no credentials (local services).
	Env []string `json:"env,omitempty"`
	// NPM is the @ai-sdk/* package name the upstream catalog uses; we
	// repurpose it as a compatibility-family hint. See FamilyFromNPM.
	NPM string `json:"npm,omitempty"`
	// API is the base URL for the provider's HTTP endpoint.
	API string `json:"api,omitempty"`
	// Doc is the URL of the provider's documentation; surfaces in
	// `agt catalog list` so operators can find it.
	Doc string `json:"doc,omitempty"`
	// Models is the catalog of models this provider serves, keyed by
	// model ID.
	Models map[string]*Model `json:"models,omitempty"`
}

// HasCredentials reports whether any of the configured env-var names
// is set in env. Used by the Governor to filter the registry to
// providers we can actually call.
func (p *Provider) HasCredentials(lookup func(string) string) bool {
	if len(p.Env) == 0 {
		// No credentials required (local services like Ollama).
		return true
	}
	for _, name := range p.Env {
		if v := lookup(name); v != "" {
			return true
		}
	}
	return false
}

// Family is the wire-dialect family — what adapter (Anthropic Messages,
// OpenAI Chat Completions, Ollama /api/chat, etc.) the Governor uses
// to talk to this Provider.
type Family string

const (
	FamilyAnthropic        Family = "anthropic"
	FamilyOpenAI           Family = "openai"
	FamilyOpenAICompatible Family = "openai-compatible"
	FamilyGoogle           Family = "google"        // Generative Language API (API key)
	FamilyGoogleVertex     Family = "google-vertex" // Vertex AI (service-account OAuth)
	FamilyOllama           Family = "ollama"
	FamilyMistral          Family = "mistral"
	FamilyCohere           Family = "cohere"
	FamilyAWSBedrock       Family = "aws-bedrock"
	FamilyAzure            Family = "azure"
	FamilyUnknown          Family = "unknown"
)

// FamilyFromNPM maps an `@ai-sdk/*` package name to one of our known
// compat families. The mapping is conservative: anything not
// recognised falls to FamilyUnknown so the Governor can refuse it
// rather than guess wrong.
func FamilyFromNPM(npm string) Family {
	n := strings.TrimSpace(strings.ToLower(npm))
	// Handle non-Vercel namespaces explicitly before stripping the
	// @ai-sdk/ prefix — they don't share it.
	switch n {
	case "@openrouter/ai-sdk-provider":
		return FamilyOpenAICompatible
	}
	n = strings.TrimPrefix(n, "@ai-sdk/")
	switch n {
	case "anthropic":
		return FamilyAnthropic
	case "openai":
		return FamilyOpenAI
	case "openai-compatible":
		return FamilyOpenAICompatible
	// First-party Vercel AI SDK packages whose wire dialect is OpenAI
	// Chat Completions — same Bearer-auth + /v1/chat/completions shape,
	// just hosted under a different base URL. The catalog entry's
	// `api` field carries that base URL.
	case "groq",
		"xai",
		"cerebras",
		"togetherai",
		"deepinfra",
		"perplexity",
		"fireworks":
		return FamilyOpenAICompatible
	case "google", "google-generative-ai":
		return FamilyGoogle
	case "google-vertex", "google-vertex/anthropic":
		return FamilyGoogleVertex
	case "ollama":
		return FamilyOllama
	case "mistral":
		return FamilyMistral
	case "cohere":
		return FamilyCohere
	case "amazon-bedrock":
		return FamilyAWSBedrock
	case "azure":
		return FamilyAzure
	}
	return FamilyUnknown
}

// Family is the resolved Family for this provider, derived from NPM.
func (p *Provider) Family() Family { return FamilyFromNPM(p.NPM) }

// Model is one model the provider offers. Field names mirror
// models.dev/api.json. Pricing is USD per-million-tokens.
type Model struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Family     string     `json:"family,omitempty"`
	Attachment bool       `json:"attachment,omitempty"`
	Reasoning  bool       `json:"reasoning,omitempty"`
	ToolCall   bool       `json:"tool_call,omitempty"`
	Knowledge  string     `json:"knowledge,omitempty"`    // YYYY-MM
	Release    string     `json:"release_date,omitempty"` // YYYY-MM-DD
	Modalities Modalities `json:"modalities,omitempty"`
	OpenWeight bool       `json:"open_weights,omitempty"`
	Limit      Limit      `json:"limit,omitempty"`
	// Cost is omitted for free/local models (Ollama, self-hosted).
	Cost *Cost `json:"cost,omitempty"`
}

// Modalities captures the I/O surfaces the model supports.
type Modalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// Limit captures the model's token windows.
type Limit struct {
	Context int `json:"context,omitempty"`
	Output  int `json:"output,omitempty"`
	Input   int `json:"input,omitempty"`
}

// Cost is USD per-million-tokens. Cache fields are optional and only
// populated for providers that surface prompt-caching prices
// separately (Anthropic, OpenAI).
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// InputMicrocentsPerMTok returns the input price in our internal unit
// (USD-microcents per million tokens). $5/MTok → 5_000_000_000.
//
// Conversion: 1 USD = 100 cents = 100 × 10_000_000 microcents = 10^9.
// So price_usd_per_MTok × 10^9 = microcents per MTok.
func (c *Cost) InputMicrocentsPerMTok() int64 {
	if c == nil {
		return 0
	}
	return int64(c.Input * 1_000_000_000)
}

// OutputMicrocentsPerMTok is the output price in USD-microcents per MTok.
func (c *Cost) OutputMicrocentsPerMTok() int64 {
	if c == nil {
		return 0
	}
	return int64(c.Output * 1_000_000_000)
}

// Catalog is the in-memory union of every loaded source (api.json +
// local.json + custom.json), with custom > local > api precedence
// already applied. Safe to read concurrently; rebuild with Reload.
type Catalog struct {
	// Providers indexed by ID. Concurrent reads are safe; do not
	// mutate after construction.
	Providers map[string]*Provider
	// SyncedAt is when the most-recent successful sync wrote
	// api.json. Zero value if never synced.
	SyncedAt time.Time
	// Sources is the list of files that contributed to this catalog,
	// in the order they were merged (api, local, custom).
	Sources []string
}

// NewEmpty returns an empty Catalog ready to be merged into.
func NewEmpty() *Catalog {
	return &Catalog{Providers: map[string]*Provider{}}
}

// ProviderList returns providers sorted by ID for stable iteration.
func (c *Catalog) ProviderList() []*Provider {
	out := make([]*Provider, 0, len(c.Providers))
	for _, p := range c.Providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FindModel returns the (Provider, Model) pair for a model ID. Search
// is two-pass: exact provider/model first when modelID contains "/"
// (e.g. "anthropic/claude-opus-4-5"); otherwise scan every provider
// for a matching model ID and return the first hit (deterministic via
// sorted provider iteration). Returns (nil, nil) if not found.
func (c *Catalog) FindModel(modelID string) (*Provider, *Model) {
	if modelID == "" {
		return nil, nil
	}
	if idx := strings.Index(modelID, "/"); idx > 0 {
		provID := modelID[:idx]
		mID := modelID[idx+1:]
		if p, ok := c.Providers[provID]; ok {
			if m, ok := p.Models[mID]; ok {
				return p, m
			}
		}
		return nil, nil
	}
	for _, p := range c.ProviderList() {
		if m, ok := p.Models[modelID]; ok {
			return p, m
		}
	}
	return nil, nil
}

// Merge folds src into dst with src winning on key conflict. Mutates
// dst.Providers in place; per-provider Models maps are also merged
// (src model wins). Used by the loader to apply local/custom on top
// of api.json.
func (dst *Catalog) Merge(src *Catalog) {
	for id, sp := range src.Providers {
		if existing, ok := dst.Providers[id]; ok {
			// Provider exists; merge model maps and prefer src for
			// non-model fields if they're populated.
			if sp.Name != "" {
				existing.Name = sp.Name
			}
			if len(sp.Env) > 0 {
				existing.Env = sp.Env
			}
			if sp.NPM != "" {
				existing.NPM = sp.NPM
			}
			if sp.API != "" {
				existing.API = sp.API
			}
			if sp.Doc != "" {
				existing.Doc = sp.Doc
			}
			if existing.Models == nil {
				existing.Models = map[string]*Model{}
			}
			for mid, m := range sp.Models {
				existing.Models[mid] = m
			}
		} else {
			// Copy so callers can't mutate src.
			cp := *sp
			cp.Models = map[string]*Model{}
			for mid, m := range sp.Models {
				cp.Models[mid] = m
			}
			dst.Providers[id] = &cp
		}
	}
}

// ParseAPIFile parses a models.dev-shaped api.json into a Catalog.
// Returns an error if the JSON is malformed or has the wrong shape.
func ParseAPIFile(raw []byte) (*Catalog, error) {
	var byID map[string]*Provider
	if err := json.Unmarshal(raw, &byID); err != nil {
		return nil, fmt.Errorf("catalog: parse api.json: %w", err)
	}
	c := NewEmpty()
	for id, p := range byID {
		if p == nil {
			continue
		}
		if p.ID == "" {
			p.ID = id
		}
		c.Providers[p.ID] = p
	}
	return c, nil
}

// MarshalAPI returns the JSON form (models.dev shape) for the catalog,
// suitable for writing back to disk.
func (c *Catalog) MarshalAPI() ([]byte, error) {
	return json.MarshalIndent(c.Providers, "", "  ")
}
