// SPDX-License-Identifier: MIT

// Package llm is the model-provider contract: the canonical conversation
// (Message, Role, ToolCall), one completion round trip (CompletionRequest,
// CompletionResponse, Usage, StopReason, Params) and the provider interfaces
// (Provider, StreamingProvider, Chunk). Pure types — accessors only, standard
// library plus the sibling tool contract — so any layer may depend on it
// (architecture/20-target-architecture.md §2, layer L1).
//
// These types used to live in kernel/agent, next to the agent loop, so the
// model gateway, every provider adapter and every package that merely made an
// LLM call depended on the loop itself. kernel/agent keeps type aliases, so
// existing importers are unaffected.
package llm

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Role is the canonical conversation role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one canonical conversation turn.
//
// For role=assistant, the model may return either Content (final text),
// ToolCalls (a request to invoke one or more tools), or both. For
// role=tool, ToolCallID identifies which assistant ToolCall this responds
// to, and Content carries the textual result.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// Images carries image-attachment references for a vision-capable run
	// (M93). Additive + omitempty — providers that don't read it are
	// unaffected, and the M91 capability gate ensures a non-vision model never
	// receives a message carrying images.
	Images []string `json:"images,omitempty"`
}

// ToolCall is a model-issued request to invoke a tool.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// CompletionRequest is what the loop sends to a Provider.
type CompletionRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []toolapi.ToolDef
	MaxTokens int
	// TaskType is an optional hint to the Governor's per-task-type
	// routing layer (M1.cc). Free-form string; callers set it to
	// classify what kind of work this completion is — e.g. "plan"
	// for planner LLM calls, "salience" for memory-pruning calls,
	// "code" for code-gen-heavy work. Empty (the default) means
	// "no hint; use the standard subscription-first routing chain."
	//
	// The string is opaque to providers — they don't see it. Only
	// the Governor consults it, and only when the operator has
	// configured TaskRoutes for the key. So providers and tests
	// can ignore the field; setting or not setting it never changes
	// what the provider receives.
	TaskType string
	// ModelChain is an optional per-REQUEST ordered model fallback chain
	// (M787): when set, a chain-aware router (the Governor) tries these
	// models in order and it WINS over the task type's configured chain.
	// Carries a named agent's own fallbacks (roster M783). Like TaskType it
	// is a Governor-only hint — plain providers never consult it.
	ModelChain []string
	// Agent + AgentDailyCeilingMc carry a named agent's identity and its
	// per-day spend ceiling (roster MaxDailyMc, M793). Governor-only: the
	// Governor keeps a per-agent daily ledger and refuses completions past
	// the ceiling; plain providers never consult either field.
	Agent               string
	AgentDailyCeilingMc int64
	// CorrelationID identifies the run this completion serves. Like
	// TaskType it is a Governor-only hint — opaque to providers, who
	// never see it — letting the Governor stamp its budget.consumed
	// event with the spending run's correlation (M47) so spend can be
	// attributed per run / per delegation. Empty means "unattributed".
	CorrelationID string
	// JSONMode requests structured (JSON) output from the model — the
	// "reliability over free-form parsing" path of SPEC-10 §2, used by
	// callers that must parse the result (plan generation, classifications).
	// Providers with a native JSON mode honour it (OpenAI response_format,
	// Gemini responseMimeType, Ollama format=json); providers without one
	// ignore it (the caller keeps its robust prompt-based parsing). Default
	// false leaves every request byte-for-byte unchanged.
	JSONMode bool
	// Params carries optional universal sampling knobs (temperature, top_p,
	// seed, stop, penalties, reasoning effort). Zero value (Params.IsZero())
	// means "unset" and every adapter leaves the wire request unchanged.
	// Unlike the Governor-only hints above, providers DO consult Params.
	Params Params
	// ProviderOptions carries provider-specific extras that don't generalise,
	// keyed by provider family or registry name (e.g. "anthropic", "openai").
	// Each adapter reads only its own key and merges the raw JSON object into
	// the outbound request. A nil map (the default) changes nothing.
	ProviderOptions map[string]json.RawMessage
}

// StopReason is the canonical reason a Provider stopped emitting tokens.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
)

// CompletionResponse is what a Provider returns from Complete.
type CompletionResponse struct {
	Message    Message // role=assistant; may contain ToolCalls
	StopReason StopReason
	Usage      Usage
	// ReasoningContent is the model's reasoning / chain of thought, when a
	// reasoning model returns it separately from the answer (M317; DeepSeek-R1
	// and compatible models' `reasoning_content`). Empty for ordinary models.
	// Surfaced live as ephemeral llm.reasoning events and as a char count on the
	// durable llm.response event — the full text is not journaled (it can be very
	// large, and the answer is what the audit chain needs).
	ReasoningContent string
}

// Usage carries per-call token accounting (cost translation lives in the
// Governor at MVP time, SPEC-10; M0.5 just records the raw tokens).
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	// CachedInputTokens is the subset of InputTokens that hit the provider's
	// prompt cache (0 when the provider reports none / doesn't support it).
	// Billed at the model's cache-read rate; see governor.costMicrocentsCached.
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	// CacheWriteInputTokens is the subset of InputTokens written into the
	// provider's prompt cache this call (Anthropic's cache_creation_input_tokens;
	// 0 for providers without a separate cache-write count). Billed at the
	// model's cache-write rate (typically a premium over input).
	CacheWriteInputTokens int    `json:"cache_write_input_tokens,omitempty"`
	Model                 string `json:"model,omitempty"`
}

// Params carries optional per-request sampling / generation knobs that are
// universal across providers. Every field is a pointer (or a nil-able slice)
// so the zero value means "unset — send nothing, let the provider use its own
// default". An adapter MUST only emit a wire field when the corresponding
// pointer is non-nil, keeping an unset Params byte-for-byte identical to the
// pre-Params request (the same default-preserving contract as JSONMode).
//
// Provider-specific knobs that don't generalise (e.g. Anthropic's raw thinking
// config) ride CompletionRequest.ProviderOptions instead; ReasoningEffort is
// the one reasoning knob normalised here because every reasoning-capable family
// exposes some form of it (OpenAI reasoning_effort, Anthropic/Gemini thinking
// budget).
type Params struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	Stop             []string `json:"stop,omitempty"`
	Seed             *int64   `json:"seed,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	// ReasoningEffort is the normalised reasoning/thinking knob: one of
	// "", "minimal", "low", "medium", "high". Empty leaves the provider's
	// construction-time default (e.g. AGEZT_ANTHROPIC_THINKING_BUDGET) in force.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// IsZero reports whether no per-request knob is set, so adapters can cheaply
// skip the whole apply path and guarantee an unchanged request.
func (p Params) IsZero() bool {
	return p.Temperature == nil && p.TopP == nil && p.TopK == nil &&
		len(p.Stop) == 0 && p.Seed == nil && p.FrequencyPenalty == nil &&
		p.PresencePenalty == nil && p.ReasoningEffort == ""
}

// Provider is implemented by anything that can drive a chat completion.
type Provider interface {
	// Name identifies the provider plugin (e.g. "anthropic", "mock").
	Name() string
	// Complete sends one request and returns one response. Implementations
	// must honor ctx cancellation.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// StreamingProvider is the optional sibling of Provider for adapters
// that can emit partial output as the model generates it. Providers
// MUST implement Provider; implementing StreamingProvider on top is
// additive — callers type-assert to check.
//
//	if sp, ok := prov.(llm.StreamingProvider); ok {
//	    resp, err := sp.CompleteStream(ctx, req, onChunk)
//	} else {
//	    resp, err := prov.Complete(ctx, req)
//	}
//
// CompleteStream MUST return the same CompletionResponse that Complete
// would have returned for the same request — token usage, final
// assembled message, stop reason. The onChunk callback is for
// progressive UI; the canonical answer still flows through the
// returned response. This invariant lets callers safely fall back to
// Complete-style accounting (Governor pricing, journal usage stats)
// without worrying about partial state.
//
// Cancellation: ctx.Err() cancels the underlying HTTP stream. The
// onChunk callback may return an error to abort the stream early; in
// that case CompleteStream returns (nil, that error) and the
// provider is responsible for closing the HTTP connection.
type StreamingProvider interface {
	Provider
	// CompleteStream is like Complete but invokes onChunk for each
	// incremental piece of output as it arrives. onChunk MUST NOT
	// block for long — providers typically call it on the read
	// goroutine. Returning a non-nil error from onChunk aborts the
	// stream.
	CompleteStream(ctx context.Context, req CompletionRequest, onChunk func(Chunk) error) (*CompletionResponse, error)
}

// Chunk is one streamed unit. Exactly one of TextDelta /
// ToolUseStart / ToolInputJSONDelta / ToolUseStop is populated per
// chunk; the others are zero. Providers SHOULD coalesce small text
// fragments where convenient — operators want progress visibility,
// not literal one-byte-per-event.
type Chunk struct {
	// TextDelta is the next slice of assistant text. Concatenating
	// all TextDelta values across a stream reconstructs the final
	// text. Empty when this chunk carries a tool event instead.
	TextDelta string

	// ToolUseStart announces a new tool call starting. The ID +
	// Name are final; ToolInputJSONDelta chunks for this call will
	// follow until ToolUseStop. UIs typically render
	// "→ calling tool X..." at this point.
	ToolUseStart *ToolCall

	// ToolInputJSONDelta is the next slice of streamed JSON input
	// for the currently-open tool call. Anthropic and OpenAI both
	// stream tool inputs as incremental JSON fragments rather than
	// complete objects. Concatenating all deltas for one ID yields
	// the final input JSON.
	ToolInputJSONDelta string

	// ToolUseStop signals the currently-streaming tool call's
	// input is complete. ID identifies which call; UIs can finalize
	// any per-call progress widget here.
	ToolUseStop string

	// ReasoningDelta is the next slice of the model's reasoning / chain of
	// thought (M317), for reasoning models that stream it separately from the
	// answer (DeepSeek-R1 and compatible models' `reasoning_content`).
	// Concatenating all ReasoningDelta values reconstructs the full reasoning.
	// Empty for non-reasoning models and for tool/text chunks.
	ReasoningDelta string
}

// IsEmpty reports whether the chunk carries no signal. Stream
// implementations may emit periodic keep-alives that decode to
// empty Chunks; callers can use this to skip them.
func (c Chunk) IsEmpty() bool {
	return c.TextDelta == "" &&
		c.ToolUseStart == nil &&
		c.ToolInputJSONDelta == "" &&
		c.ToolUseStop == ""
}
