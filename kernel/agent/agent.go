// SPDX-License-Identifier: MIT

// Package agent defines the canonical, dialect-free conversation/tool types
// and the first-party single-agent tool-loop (DECISIONS B0d).
//
// The loop is owned end-to-end by Agezt — no third-party agent SDK
// (deliberate, non-negotiable). Provider plugins translate the canonical
// Message / ToolCall shapes to/from their backend dialect
// (Anthropic / OpenAI / Gemini / ...; SPEC-15).
//
// Every step the loop takes is journaled via the bus
// (durable-before-publish). Bounded by MaxIter and honors context
// cancellation (which is how `agt halt` stops a run).
package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/bus"
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

// LoopConfig configures one tool-loop run.
type LoopConfig struct {
	Provider Provider
	Tools    map[string]Tool
	Bus      *bus.Bus
	Model    string
	System   string
	// MaxIter caps tool-call rounds (DECISIONS E5: default 25).
	MaxIter int
	// MaxAutoContinue caps how many times the loop AUTOMATICALLY continues a run
	// that exhausted MaxIter without a final answer (M833): instead of failing
	// with ErrMaxIter, it injects a "keep going" turn and grants another MaxIter
	// rounds, repeating until the task completes or this cap is hit. 0 → default
	// (DefaultMaxAutoContinue); a negative value disables auto-continue (the old
	// fail-at-MaxIter behaviour). The per-run cost cap, the identical-call guard,
	// and context cancellation/timeout remain the real safety nets across
	// continuations.
	MaxAutoContinue int
	// AutoContinueWait is an optional pause before each automatic continuation
	// (M833) — a brief breather so a wedged run doesn't hammer the provider, and
	// a window for an operator to halt. 0 → DefaultAutoContinueWait. Honoured
	// ctx-aware: a cancel during the wait ends the run immediately.
	AutoContinueWait time.Duration
	// ToolTimeout, when > 0, bounds each individual tool invocation's
	// wall-clock (M34). A tool that overruns has its call context cancelled
	// and the loop feeds an IsError result ("tool X exceeded its … timeout")
	// back to the model — the RUN continues, unlike the per-run MaxDuration
	// (M31) which terminates the whole run. 0 = unbounded tool calls. A
	// genuine run-level cancel/timeout (operator halt, M32 cancel, or the
	// per-run deadline) still propagates and fails the run.
	ToolTimeout time.Duration
	// MaxTokens passed to the provider per call. 0 → provider default.
	MaxTokens int
	// JSONMode requests structured (JSON) output on every provider call of the
	// run (M314). Set by callers that need a machine-parseable result; a provider
	// without a native JSON mode ignores it. Flows to CompletionRequest.JSONMode.
	JSONMode bool
	// Params carries optional universal sampling knobs (temperature, top_p,
	// seed, stop, penalties, reasoning effort) applied to every provider call of
	// the run (M997). Zero value leaves the request unchanged. Flows to
	// CompletionRequest.Params.
	Params Params
	// TaskType is the per-run routing hint (M703) carried into every
	// CompletionRequest of the run, so the Governor's per-task-type model
	// chains / overrides / routes apply. The main chat loop sets "chat";
	// delegated sub-agents set "delegate" (or a per-delegation type). Empty →
	// no hint (default routing).
	TaskType string
	// ModelChain is the per-run ordered model fallback chain (M787) carried
	// into every CompletionRequest, overriding the task type's configured
	// chain — a named agent's own fallbacks (roster M783). Empty → none.
	ModelChain []string
	// Agent + AgentDailyCeilingMc identify the named agent this run executes
	// AS and its per-day spend ceiling (M793), carried into every request so
	// the Governor's identity ledger can meter and refuse. Empty/0 → none.
	Agent               string
	AgentDailyCeilingMc int64
	// Wake* fields describe why this run exists. They are provenance, not prompt:
	// schedule/standing/manual/sub-agent wakeups stamp task.received so every UI
	// and audit projection can explain what woke the agent without parsing intent.
	WakeSource        string
	WakeReason        string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
	// MaxParallelTools caps how many tool calls from ONE assistant response
	// execute concurrently (M880). A model that fans out several independent
	// calls in a single turn no longer waits for each to finish before the
	// next starts. Gating (loop guard, policy) and its journaling stay
	// sequential in call order, and tool.result events + tool messages are
	// emitted in the original call order afterwards — the conversation the
	// model sees is byte-identical to the sequential build. Tools are already
	// required to be goroutine-safe (the daemon invokes the same Tool values
	// from concurrent runs), so in-turn parallelism adds no new contract.
	// 0 → DefaultMaxParallelTools; 1 or negative → strictly sequential.
	MaxParallelTools int
	// MaxIdenticalToolCalls caps how many times the model may invoke the SAME
	// (tool, input) within one run before the loop refuses to execute it again
	// (M116). A model stuck retrying an identical failing/expensive call would
	// otherwise re-run it every iteration up to MaxIter; the guard stops the
	// re-execution and feeds back a clear nudge to change approach. 0 → default
	// (DefaultMaxIdenticalToolCalls); a negative value disables the guard.
	MaxIdenticalToolCalls int
	// DirectiveTaintWindow is how many iterations a directive-like untrusted
	// observation keeps the prompt-injection gate active for downstream effectful
	// actions. 0 → DefaultDirectiveTaintWindow (1). Larger values gate further
	// past the observation; the security-conscious can widen it.
	DirectiveTaintWindow int
	// ToolMemo caches successful read-only tool results within this run. Policy
	// still runs before cache lookup, so memoization never grants permission.
	ToolMemo *ToolMemo
	// ToolResultHook is called after an invoked tool has a classified result,
	// before that result is appended back to the model. It is best-effort
	// runtime bookkeeping; implementations must not panic.
	ToolResultHook func(context.Context, ToolCall, Result)
	// Actor is the journaling actor for emitted events (e.g. "agent-01H").
	Actor string
	// CorrelationID ties every event in this run together.
	CorrelationID string
	// Policy is the optional pre-tool-call gate. When non-nil, the loop
	// calls it before invoking each ToolCall and journals a policy.decision
	// event. A Deny verdict skips the tool invocation; the model sees a
	// tool result containing the deny reason so it can adjust.
	Policy Policy
	// ToolSelector optionally chooses a relevant subset of registered tools to
	// offer before each provider call (CH-03 semantic discovery). Nil preserves
	// the historical behaviour: every non-denied tool is offered.
	ToolSelector ToolSelector
	// ObservationDeltas, when true, sends repeated observations of the same
	// tool/input pair back to the model as a structured delta while keeping the
	// full raw output in the journal. Off by default for byte-for-byte
	// compatibility. (CH-04)
	ObservationDeltas bool
	// Images attaches image references to the initial user message (M93).
	// Only set on a vision-capable run (gated upstream by M91); the loop
	// puts them on the first user Message so the provider can encode them.
	Images []string
	// MaxRunCostMicrocents, when > 0, caps the cumulative provider spend for THIS
	// run (M166) — the per-run cost analogue of MaxIter (round cap) and the per-run
	// MaxDuration (wall-clock cap). After each model call the loop adds the call's
	// cost (via CostFn) and, once the running total reaches the cap, terminates the
	// run with ErrRunBudgetExceeded. 0 = uncapped. Has no effect without CostFn.
	MaxRunCostMicrocents int64
	// CostFn translates a model call's token usage to spend in microcents. Injected
	// (rather than imported) so kernel/agent stays decoupled from kernel/governor's
	// pricing; the kernel wires governor.CostMicrocents. nil disables cost
	// accounting entirely (MaxRunCostMicrocents is then inert).
	CostFn func(model string, inputTokens, outputTokens int) int64
	// Artifacts, when non-nil, offloads a tool output larger than
	// ArtifactThreshold bytes into a content-addressed store, so the journaled
	// tool.result carries a small preview + a raw_ref instead of the full bytes
	// (SPEC-04 §3.6 / SPEC-01 §10.2). The MODEL still receives the complete
	// output — only the journal event is slimmed. A Put failure falls back to
	// inlining, so storage trouble never fails a run. nil disables offload.
	Artifacts ArtifactPutter
	// ArtifactThreshold is the byte size above which a tool output is offloaded.
	// 0 with a non-nil Artifacts uses DefaultArtifactThreshold.
	ArtifactThreshold int
	// ContextBudget, when > 0, caps the assembled-context size (chars) the loop
	// sends per call (SPEC-10 §3). Before each provider call, if the context
	// exceeds the budget the loop elides the OLDEST tool outputs to stubs —
	// system prompt and the most recent turns are always kept — and journals a
	// context.compacted event. 0 disables (the historical full-history behaviour).
	ContextBudget int
	// ContextProtectLast is how many of the most-recent messages are never elided
	// by budget compaction (the model needs its latest context intact). 0 uses
	// DefaultContextProtectLast.
	ContextProtectLast int
	// ContextProtectFirst is how many of the EARLIEST messages budget compaction
	// never elides, preserving the run's original grounding (the first task
	// framing and discovery results) even as the oldest middle turns are dropped.
	// 0 keeps the historical behaviour — elide strictly oldest-first, protecting
	// only the tail. (M395)
	ContextProtectFirst int
	// SummarizeElided, when non-nil, produces a one-line summary of a tool output
	// being elided by budget compaction — an abstractive replacement for the
	// deterministic head-snippet stub (M397), so the model keeps the *meaning* of
	// the dropped output, not just its first characters. It is called at most once
	// per distinct output (the loop caches by content) and only when compaction
	// actually elides. A non-empty return is embedded in the stub; "" or an error
	// falls back to the head snippet. nil (the default) keeps the head-snippet
	// behaviour with zero extra provider calls. (M398, SPEC-10 §3)
	SummarizeElided func(ctx context.Context, toolOutput string) (string, error)
	// ContextRescueMarkers marks tool outputs that must be preserved during
	// compaction even when they are old. Runtime uses this for skill bundle reads
	// so a just-loaded procedure/resource is not summarized away before the model
	// can apply it. Empty keeps historical oldest-tool-output elision.
	ContextRescueMarkers []string

	// Steer, when non-nil, is the per-run live-steering control surface (M608).
	// At the top of each iteration the loop calls Steer.Wait (blocking while the
	// operator has paused the run) and then folds any operator-injected
	// directives from Steer.Drain into the conversation as fresh user turns — so
	// an operator can redirect or pause/step a running agent from the cockpit
	// without cancelling it. nil (the default) disables steering with zero
	// overhead. Implementations must be safe for concurrent use: the operator
	// drives them from another goroutine while the loop runs.
	Steer Steerer
	// PriorMessages, when non-empty, seeds the loop with an existing conversation
	// instead of a fresh user turn — the mechanism the daemon uses to RESUME a run
	// interrupted by a restart (M1002). The slice is the run's message history as
	// captured at a safe iteration boundary (a complete assistant→tool set), so
	// tool-call/result pairing stays consistent. Its first element is already the
	// original user-intent turn, so it REPLACES that turn (the loop does not
	// prepend a second). nil (the default) is a normal fresh run.
	PriorMessages []Message
	// StartIter is the iteration number a resumed run continues from, so journal
	// iter numbers keep climbing and the resumed run gets a full fresh MaxIter
	// budget from that point. 0 for a fresh run.
	StartIter int
	// Checkpoint, when set, is called at the top of every iteration (the same safe
	// boundary Steer uses) with the current iteration number and the conversation
	// so far. The daemon persists it (M1002) so the run can resume after a restart.
	// Called on the loop goroutine — the implementation MUST be cheap and
	// non-blocking, and MUST copy the slice if it retains it (the loop keeps
	// appending to the passed messages). nil disables checkpointing with zero
	// overhead.
	Checkpoint func(iter int, messages []Message)
}

// Steerer is the optional per-run control surface the loop consults at the top
// of every iteration (M608). It lets an operator fly a running agent — inject
// guidance, pause, single-step, resume — without cancelling it. The kernel
// supplies the live implementation (kernel/runtime); tests can supply a fake.
type Steerer interface {
	// Wait blocks while the run is paused and returns when it is resumed,
	// single-stepped, or ctx is done. It returns ctx.Err() if the context ends
	// while waiting (so a paused run still honours halt/cancel/timeout), and nil
	// otherwise. When the run is not paused it returns immediately.
	Wait(ctx context.Context) error
	// Drain returns and clears any directives the operator has injected since the
	// last call, in submission order. The loop appends each as a user turn before
	// the next model call. Returns nil when none are pending.
	Drain() []Directive
}

// Directive is one operator injection the loop folds into the run at a safe
// boundary (M962). Note distinguishes a soft "by the way" — read it, but finish
// the current step and stay on task — from a forceful steer that re-prioritises.
type Directive struct {
	Text string
	Note bool
}

// DefaultContextProtectLast is how many trailing messages context compaction
// never touches, so the model always keeps its most recent exchange whole.
const DefaultContextProtectLast = 4

// DefaultContextProtectFirst is how many leading messages context compaction
// never touches by default. 0 means protect-first is opt-in: with it unset,
// compaction elides strictly oldest-first and only the tail is shielded.
const DefaultContextProtectFirst = 0

// ContextCharsPerToken is the rough chars-per-token ratio used to translate a
// model's token context window (catalog Limit.Context) into the char-denominated
// budget the loop measures. ~4 is the common English approximation; the budget is
// a soft cap, not an exact token count, so an approximation is fine.
const ContextCharsPerToken = 4

// DefaultCompressFraction is the fraction of the model's context window at which
// auto-budgeting starts compacting (SPEC-16 §3 compress_at_fraction). Half the
// window leaves ample room for the model's own output + a safety margin.
const DefaultCompressFraction = 0.5

