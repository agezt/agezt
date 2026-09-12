// SPDX-License-Identifier: MIT

// Agent core types: Role + Role const + Message + ToolCall + ToolDef + ToolCapability + CompletionRequest + StopReason + CompletionResponse + Usage + Provider + Tool + Result.
// Code extracted from agent.go during the Day-78 god-file split. Public API unchanged.
package agent


import (
	"context"
	"encoding/json"
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

// ToolDef advertises a tool to the model.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
	Effect      ToolEffect      `json:"-"`
	// Capability declares which policy axis this tool's calls are gated on.
	// Governance metadata like Effect, so it stays off the provider wire.
	//
	// Declare it. The alternative is a name switch in the policy package, which
	// is a different package from the tool — and a tool missing from that switch
	// resolves to a capability the engine doesn't know, which is DEFAULT-DENIED.
	// That is not a degraded tool, it is a dead one, and it fails silently at run
	// time rather than at build time. Six tools shipped that way before this
	// field existed.
	Capability ToolCapability `json:"-"`
}

// ToolCapability is a tool's declared policy axis.
//
// Most tools exercise one capability for every call and set only Name. A tool
// whose risk depends on what the call ASKS FOR — reading a file versus deleting
// one — also names the input field that selects the axis and maps its values.
type ToolCapability struct {
	// Name is the axis for any call ByValue does not match.
	//
	// For a single-axis tool it is the whole declaration. For a multi-axis one it
	// is the fallback, and choosing it is a real decision the tool's author is
	// best placed to make: a reader should fall back to its READ axis so a
	// garbled call cannot gain write access, while an installer should fall back
	// to its GATED axis so a garbled call cannot slip past the grant. Leaving it
	// empty is not neutral — it defers to the policy package's name switch.
	Name string
	// Field is the top-level input field whose value picks the axis, e.g. "op",
	// "method", or "operation". Empty means single-axis.
	Field string
	// ByValue maps a Field value to its axis. Lookup trims and lower-cases, so
	// entries should be lower-case; a value absent here falls back to Name.
	ByValue map[string]string
}

// IsZero reports whether a tool declared no capability, in which case the
// caller must fall back to its own classification.

// CompletionRequest is what the loop sends to a Provider.
type CompletionRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolDef
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

// Provider is implemented by anything that can drive a chat completion. For
// M0.5 we only support non-streaming Complete; streaming lands later.
type Provider interface {
	// Name identifies the provider plugin (e.g. "anthropic", "mock").
	Name() string
	// Complete sends one request and returns one response. Implementations
	// must honor ctx cancellation.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// Tool is implemented by anything the agent loop can invoke during a
// task. In-process by default (DECISIONS B0a); out-of-process plugins
// satisfy the same interface via a thin client.
type Tool interface {
	// Definition is the schema/description advertised to the model.
	Definition() ToolDef
	// Invoke executes the tool with the parsed input and returns the
	// textual result for the model. Implementations must honor ctx.
	Invoke(ctx context.Context, input json.RawMessage) (Result, error)
}

// Result is what a Tool returns. IsError signals to the loop that the model
// should see an error (still appended as a tool result message, so the
// model can retry or adjust).
type Result struct {
	Output  string
	IsError bool
	// ObservationTrust classifies tool output before it is fed back to the
	// model. Empty means "use the loop's default for this tool". External
	// world content should be ObservationUntrusted so it is rendered as data,
	// never as an instruction channel.
	ObservationTrust ObservationTrust
	// ObservationSource names the external source in operator-facing audit
	// metadata, e.g. "https://example.com" or "workspace:file.md".
	ObservationSource string
}
