// SPDX-License-Identifier: MIT

// Anthropic provider: anth* wire types (anthRequest, anthMessage, anthTool,
// anthResponse, ...) and small helpers (applyParams, thinkingConfig,
// buildAnthSystem, buildAnthTools, anthUsageToAgent) used by Complete to
// encode/decode requests. Extracted from anthropic_wire.go during the
// Day-211 god-file split. Public API unchanged.
package anthropic


import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)
// ----- dialect translation (canonical ↔ Anthropic) -----

// anthRequest is the wire-shape of a Messages API request. System is `any` so it
// can be the plain-string form OR the block-array form that carries a prompt-cache
// breakpoint (M301); buildAnthSystem picks the shape.
type anthRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    any           `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	Tools     []anthTool    `json:"tools,omitempty"`
	Thinking  *anthThinking `json:"thinking,omitempty"` // extended thinking (M318)
	// Per-request sampling knobs (M997). Anthropic has no seed / penalties.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// applyParams copies the universal sampling knobs Anthropic understands. The
// reasoning knob is handled separately (mapped to a thinking budget) and so is
// ignored here. An unset Params leaves the request unchanged.
func (wire *anthRequest) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	wire.Temperature = p.Temperature
	wire.TopP = p.TopP
	wire.TopK = p.TopK
	wire.StopSequences = p.Stop
}

// anthThinking is Anthropic's extended-thinking request block.
type anthThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // >= 1024, and < max_tokens
}

// thinkingConfig returns the thinking block + the max_tokens to use when an
// extended-thinking budget is set. Anthropic requires max_tokens > budget_tokens
// (thinking tokens count toward the output allowance), and budget >= 1024, so we
// clamp the budget up and ensure max_tokens leaves room for the answer on top.
// budget <= 0 → (nil, maxTok unchanged): thinking off, wire byte-identical.
func thinkingConfig(budget, maxTok int) (*anthThinking, int) {
	if budget <= 0 {
		return nil, maxTok
	}
	if budget < MinThinkingBudget {
		budget = MinThinkingBudget
	}
	if maxTok <= budget {
		maxTok = budget + DefaultMaxTokens // room for the answer beyond the thinking
	}
	return &anthThinking{Type: "enabled", BudgetTokens: budget}, maxTok
}

// anthSystemBlock is one element of the array form of the system prompt — used
// when caching so the (large, stable) system prompt carries a cache_control
// breakpoint. Anthropic caches the prefix tools→system, so marking the system
// block caches tools AND system, the whole stable prefix of an agent loop.
type anthSystemBlock struct {
	Type         string            `json:"type"` // "text"
	Text         string            `json:"text"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

// buildAnthSystem returns the system field: nil when empty (omitted), else a
// one-element block array with a cache_control breakpoint (M301). Anthropic
// accepts both the string and the array form; the array form lets the stable
// system prompt be cached alongside the tools (cache reads bill at ~0.1× input).
func buildAnthSystem(system string) any {
	if system == "" {
		return nil
	}
	return []anthSystemBlock{{Type: "text", Text: system, CacheControl: &anthCacheControl{Type: "ephemeral"}}}
}

type anthTool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

// anthCacheControl marks a content/tool block as a prompt-cache breakpoint
// (M299). Anthropic caches the request prefix up to and including the marked
// block; "ephemeral" is the 5-minute TTL tier.
type anthCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// buildAnthTools converts canonical tool defs to Anthropic's wire shape and marks
// the LAST tool with cache_control (M299). Anthropic caches the prefix up to and
// including the marked block, so this caches the whole tools array — the large,
// stable part of an agent loop's request that repeats every iteration. Anthropic
// silently ignores the marker when the prefix is below the minimum cacheable size
// (so it's safe to always set), and cache reads bill at ~0.1× input (M289-291),
// turning the repeated tools into a real saving (surfaced by `agt cache`).
func buildAnthTools(tools []agent.ToolDef, fwd map[string]string) []anthTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthTool{Name: toolname.Wire(fwd, t.Name), Description: t.Description, InputSchema: t.InputSchema})
	}
	out[len(out)-1].CacheControl = &anthCacheControl{Type: "ephemeral"}
	return out
}

type anthMessage struct {
	Role    string      `json:"role"` // "user" | "assistant"
	Content []anthBlock `json:"content"`
}

// anthBlock is a single content block. Only one of Text / ToolUse /
// ToolResult is populated per block; the wire format is a tagged union.
type anthBlock struct {
	Type       string           `json:"type"`
	Text       string           `json:"text,omitempty"`
	ID         string           `json:"id,omitempty"`          // tool_use
	Name       string           `json:"name,omitempty"`        // tool_use
	Input      json.RawMessage  `json:"input,omitempty"`       // tool_use
	ToolUseID  string           `json:"tool_use_id,omitempty"` // tool_result
	ResultBody string           `json:"content,omitempty"`     // tool_result (string form)
	IsError    bool             `json:"is_error,omitempty"`    // tool_result
	Source     *anthImageSource `json:"source,omitempty"`      // type=image
	Thinking   string           `json:"thinking,omitempty"`    // type=thinking (M318)
}

// anthImageSource is the base64 payload of a type=image content block (M241).
type anthImageSource struct {
	Type      string `json:"type"`       // always "base64"
	MediaType string `json:"media_type"` // image/png | image/jpeg | image/gif | image/webp
	Data      string `json:"data"`       // base64-encoded image bytes
}

type anthResponse struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Role       string      `json:"role"`
	Content    []anthBlock `json:"content"`
	Model      string      `json:"model"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// anthUsageToAgent maps Anthropic's split token counts to the canonical
// agent.Usage (M290). Anthropic reports input_tokens EXCLUDING cached prompt
// tokens, with cache_read_input_tokens and cache_creation_input_tokens reported
// separately — so the real prompt size is their sum. Cache reads are marked
// cached (billed at the cheaper cache-read rate, M289) and cache-creation as
// cache-write (billed at the cache-write premium, M291). Before this, the two
// cache counts were dropped, so cached prompt tokens were billed at zero (an
// under-count when caching was on).
func anthUsageToAgent(inputTokens, cacheRead, cacheCreation, outputTokens int, model string) agent.Usage {
	return agent.Usage{
		InputTokens:           inputTokens + cacheRead + cacheCreation,
		CachedInputTokens:     cacheRead,
		CacheWriteInputTokens: cacheCreation,
		OutputTokens:          outputTokens,
		Model:                 model,
	}
}

