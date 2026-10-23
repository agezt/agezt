// SPDX-License-Identifier: MIT

package catalogapi

import (
	"fmt"
	"strings"
)

// Model is one model the provider offers. Field names mirror
// models.dev/api.json. Pricing is USD per-million-tokens.
type Model struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Family     string `json:"family,omitempty"`
	Attachment bool   `json:"attachment,omitempty"`
	Reasoning  bool   `json:"reasoning,omitempty"`
	ToolCall   bool   `json:"tool_call,omitempty"`
	// StrictToolArgs means the provider/model advertises native enforcement of
	// the declared tool-argument schema. SchemaConstrainedDecoding and
	// GrammarConstrainedDecoding are lower-level sampler capabilities that can
	// also make invalid tool arguments architecturally impossible.
	StrictToolArgs             bool       `json:"strict_tool_args,omitempty"`
	SchemaConstrainedDecoding  bool       `json:"schema_constrained_decoding,omitempty"`
	GrammarConstrainedDecoding bool       `json:"grammar_constrained_decoding,omitempty"`
	Knowledge                  string     `json:"knowledge,omitempty"`    // YYYY-MM
	Release                    string     `json:"release_date,omitempty"` // YYYY-MM-DD
	Modalities                 Modalities `json:"modalities,omitempty"`
	OpenWeight                 bool       `json:"open_weights,omitempty"`
	Limit                      Limit      `json:"limit,omitempty"`
	// Cost is omitted for free/local models (Ollama, self-hosted).
	Cost *Cost `json:"cost,omitempty"`
}

// Modalities captures the I/O surfaces the model supports.
type Modalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// SupportsModality reports whether the model lists name among its input
// (io = "input") or output (io = "output") modalities. Match is
// case-insensitive. Unknown io values return false.
func (m *Model) SupportsModality(io, name string) bool {
	var list []string
	switch strings.ToLower(io) {
	case "input":
		list = m.Modalities.Input
	case "output":
		list = m.Modalities.Output
	default:
		return false
	}
	for _, v := range list {
		if strings.EqualFold(v, name) {
			return true
		}
	}
	return false
}

// SupportsVision reports whether the model accepts image input — the
// most-asked capability. models.dev uses "image"; some catalogs say
// "vision", so both are accepted.
func (m *Model) SupportsVision() bool {
	return m.SupportsModality("input", "image") || m.SupportsModality("input", "vision")
}

// SupportsPromptCache reports whether this model advertises prompt caching — the
// signal Agezt's cache-aware billing uses (SPEC-15 §1.2 / SPEC-10 §2): a model
// carries a separate cache-read price exactly when its provider caches stable
// prompt prefixes (Anthropic, OpenAI auto-cache, Gemini, …). Free/local models
// with no Cost block report false.
func (m *Model) SupportsPromptCache() bool {
	return m.Cost.CacheReadMicrocentsPerMTok() > 0
}

// SupportsStrictToolArgs reports whether tool arguments can be constrained at
// generation time for this model. It is intentionally conservative: plain
// tool-use does not imply sampler-level schema enforcement, so models only
// return true when the catalog advertises one of the strict/constrained flags.
func (m *Model) SupportsStrictToolArgs() bool {
	if m == nil || !m.ToolCall {
		return false
	}
	return m.StrictToolArgs || m.SchemaConstrainedDecoding || m.GrammarConstrainedDecoding
}

// AgentWarnings returns operator-facing advisories about a model's
// fitness for the tool-driven agent loop. Empty slice = no concerns.
// The headline check is tool-use: an agent that can't call tools can't
// act, so a model that doesn't advertise it is the single most useful
// thing to surface before someone relies on it. The wording says
// "advertise" because the signal is catalog metadata — a locally-served
// model may in fact support tools without the catalog knowing.
func (m *Model) AgentWarnings() []string {
	var w []string
	if !m.ToolCall {
		w = append(w, fmt.Sprintf("model %q does not advertise tool-use (tool_call=false) — "+
			"the agent loop relies on tools to act; tool calls may fail or be ignored", m.ID))
	}
	if m.Limit.Context > 0 && m.Limit.Context < 8192 {
		w = append(w, fmt.Sprintf("model %q has a small context window (%d tokens) — "+
			"long agent runs with memory/tools may overflow it", m.ID, m.Limit.Context))
	}
	return w
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

// CacheReadMicrocentsPerMTok is the prompt-cache read price in USD-microcents
// per MTok (0 when the provider/model has no separate cache price). A cached
// prompt token is billed at this rate instead of the full input rate.
func (c *Cost) CacheReadMicrocentsPerMTok() int64 {
	if c == nil {
		return 0
	}
	return int64(c.CacheRead * 1_000_000_000)
}

// CacheWriteMicrocentsPerMTok is the prompt-cache write (creation) price in
// USD-microcents per MTok (0 when the model has no separate cache-write price).
// A token written into the cache is billed at this rate (typically a premium
// over the input rate).
func (c *Cost) CacheWriteMicrocentsPerMTok() int64 {
	if c == nil {
		return 0
	}
	return int64(c.CacheWrite * 1_000_000_000)
}
