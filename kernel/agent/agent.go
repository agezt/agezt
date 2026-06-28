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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/apperrors"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
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
}

// EffectClass classifies a tool call by operational reversibility. It is
// governance metadata, not provider-facing schema; runtime uses it to build HITL
// decision bundles and future compensation routing.
type EffectClass string

const (
	EffectUnknown      EffectClass = ""
	EffectReadOnly     EffectClass = "read_only"
	EffectReversible   EffectClass = "reversible"
	EffectCompensable  EffectClass = "compensable"
	EffectIrreversible EffectClass = "irreversible"
)

// ToolEffect is optional governance metadata supplied by a tool definition.
// Empty fields are filled by runtime capability defaults where possible.
type ToolEffect struct {
	Class             EffectClass
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
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

// AutoContextBudgetChars derives a char budget from a model's token context
// window: compress at half the window, ~4 chars/token. Returns 0 for an unknown
// (non-positive) window so the caller leaves compaction off rather than guessing.
func AutoContextBudgetChars(contextTokens int) int {
	if contextTokens <= 0 {
		return 0
	}
	return int(float64(contextTokens) * ContextCharsPerToken * DefaultCompressFraction)
}

// elidedStubPrefix marks a tool message whose output was dropped by context
// compaction, so a later pass doesn't re-elide it (and an operator recognises it).
const elidedStubPrefix = "[tool output elided to fit context budget"

// elidedHeadSnippetChars bounds the extractive preview kept in an elision stub —
// enough to recognise the dropped output, small enough that eliding still
// reclaims meaningful space for any output worth eliding.
const elidedHeadSnippetChars = 80

// elidedSummaryChars bounds the abstractive summary embedded in an elision stub
// (M398). A touch longer than the head snippet — a summary earns the space — but
// still capped so the stub stays small.
const elidedSummaryChars = 160

// DefaultContextRescueMarker is the stable marker a tool can include in its
// textual result to request preservation across compaction. It is deliberately
// namespaced so ordinary tool JSON does not trip it accidentally.
const DefaultContextRescueMarker = "_agezt_context_rescue"

// headSnippet returns the first n characters of s with internal whitespace runs
// collapsed to single spaces, suffixed with "…" when truncated. It is the
// extractive preview embedded in a compaction stub (M397): deterministic,
// dependency-free, and single-line so it can't break the stub it sits in.
func headSnippet(s string, n int) string {
	collapsed := strings.Join(strings.Fields(s), " ")
	r := []rune(collapsed)
	if len(r) <= n {
		return collapsed
	}
	return string(r[:n]) + "…"
}

type compactionStats struct {
	Elided       int
	Reclaimed    int
	Rescued      int
	RescuedChars int
}

func compactMessagesDetailed(system string, messages []Message, budget, protectLast, protectFirst int, summarize func(string) string, rescueMarkers []string) (out []Message, stats compactionStats) {
	if budget <= 0 {
		return messages, stats
	}
	total, _ := contextSize(system, messages)
	if total <= budget {
		return messages, stats
	}
	if protectLast <= 0 {
		protectLast = DefaultContextProtectLast
	}
	if protectFirst < 0 {
		protectFirst = 0
	}
	out = make([]Message, len(messages))
	copy(out, messages)
	limit := len(out) - protectLast // indices [start,limit) are elidable
	start := protectFirst           // indices [0,start) are protected grounding
	for i := start; i < limit && total > budget; i++ {
		m := out[i]
		if m.Role != RoleTool || m.Content == "" || strings.HasPrefix(m.Content, elidedStubPrefix) {
			continue
		}
		if rescuedToolOutput(m.Content, rescueMarkers) {
			stats.Rescued++
			stats.RescuedChars += len(m.Content)
			continue
		}
		orig := len(m.Content)
		// Prefer an abstractive one-line summary of the dropped output when a
		// summarizer is wired (M398); otherwise keep a short extractive preview of
		// the head (M397). Either way the model retains a hint of what was dropped
		// rather than a bare byte count. %q keeps the inset single-line and escaped;
		// the constant prefix preserves idempotency.
		var stub string
		if summarize != nil {
			if s := strings.TrimSpace(summarize(m.Content)); s != "" {
				stub = fmt.Sprintf("%s: %d chars · summary: %q]", elidedStubPrefix, orig, headSnippet(s, elidedSummaryChars))
			}
		}
		if stub == "" {
			stub = fmt.Sprintf("%s: %d chars · head: %q]", elidedStubPrefix, orig, headSnippet(m.Content, elidedHeadSnippetChars))
		}
		if len(stub) >= orig {
			continue // already small — eliding wouldn't help
		}
		out[i].Content = stub
		delta := orig - len(stub)
		stats.Reclaimed += delta
		total -= delta
		stats.Elided++
	}
	return out, stats
}

func rescuedToolOutput(content string, markers []string) bool {
	for _, marker := range markers {
		if marker != "" && strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

// ArtifactPutter is the slice of a content-addressed store the loop needs to
// offload large outputs. kernel/artifact.Store satisfies it. An interface keeps
// kernel/agent decoupled from the storage package.
type ArtifactPutter interface {
	Put(data []byte) (ref string, err error)
}

// DefaultArtifactThreshold is the tool-output size above which the journal event
// offloads to the artifact store (the model still sees the full output). 8 KiB
// keeps ordinary results inline while bounding the event for big dumps.
const DefaultArtifactThreshold = 8 << 10

// artifactPreviewBytes is how much of an offloaded output stays inline on the
// event as a human-readable preview.
const artifactPreviewBytes = 512

// offloadToolOutput decides how a tool output is represented ON THE EVENT. When a
// store is configured and the output exceeds the threshold, it stores the full
// bytes and returns a preview + ref + true; otherwise (no store, small output, or
// a Put error) it returns the output unchanged and offloaded=false. It never
// returns an error — offload is best-effort and must not fail the run.
func offloadToolOutput(store ArtifactPutter, threshold int, output string) (eventOutput, rawRef string, fullBytes int, offloaded bool) {
	fullBytes = len(output)
	if store == nil {
		return output, "", fullBytes, false
	}
	if threshold <= 0 {
		threshold = DefaultArtifactThreshold
	}
	if fullBytes <= threshold {
		return output, "", fullBytes, false
	}
	ref, err := store.Put([]byte(output))
	if err != nil || ref == "" {
		return output, "", fullBytes, false // fall back to inlining
	}
	preview := output
	if len(preview) > artifactPreviewBytes {
		preview = preview[:artifactPreviewBytes] + "…[offloaded; full output in artifact " + ref + "]"
	}
	return preview, ref, fullBytes, true
}

// PolicyVerdict is the contract between the loop and the policy engine
// (kernel/edict). The loop journals it as the payload of policy.decision.
type PolicyVerdict struct {
	// Allow is true → the tool will be invoked.
	Allow bool
	// Capability is the policy-engine's classification of this call
	// (e.g. "shell", "file.write", "http.post"). Free-form string so the
	// loop need not import kernel/edict.
	Capability string
	// Reason is human-readable; used in journal entries and (on deny) in
	// the synthetic tool result returned to the model.
	Reason string
	// WouldAsk indicates an approval would have been requested in a future
	// release with live HITL routing. Captured for audit.
	WouldAsk bool
	// HardDenied indicates a non-overridable rule fired.
	HardDenied bool
	// EffectClass is the runtime/tool classification used for governance
	// routing. Empty means unknown.
	EffectClass string
	// AffectedResources is a compact operator-facing resource list, when known.
	AffectedResources []string
	// EpistemicAction is the system-level calibration verdict for this proposed
	// tool call: allow, escalate, or deny. It is advisory unless the runtime
	// explicitly wires escalation into HITL.
	EpistemicAction string
	// EpistemicReason explains the calibration verdict in operator-facing text.
	EpistemicReason string
	// EpistemicSignals are structured reasons such as temporal_sensitive,
	// matched_failure_conditions:N, or low_effect_confidence:X.
	EpistemicSignals []string
	// EpistemicConfidence is the runtime confidence in the effect prediction.
	EpistemicConfidence float64
	// FailureMatches counts historical failures whose conditions match this call.
	FailureMatches int
	// WeightedFailures is FailureMatches with time decay applied.
	WeightedFailures float64
	// SchemaHash and InputShape identify the validated call condition used for
	// historical failure matching without storing raw schema/input twice.
	SchemaHash string
	InputShape string
	// TemporalSensitive marks calls whose correctness depends on fresh external
	// state, dates, versions, prices, schedules, or similar changing facts.
	TemporalSensitive bool
	// NovelTool marks calls whose tool/schema conditions have not appeared in the
	// journal window used by the runtime epistemic gate.
	NovelTool bool
	// UntrustedObservation marks proposals made after external-world data entered
	// the model context. This signal is produced by the loop, not by the model.
	UntrustedObservation bool
	// ObservationSources lists the external observation sources currently tainting
	// the run. Used for audit and HITL prompts.
	ObservationSources []string
	// ObservationDirectiveLike indicates that the external data contained text
	// resembling hidden instructions or social-engineering attempts.
	ObservationDirectiveLike bool
	// ObservationDirectiveMatches lists the directive-like patterns that fired.
	ObservationDirectiveMatches []string
}

// Policy is the signature the loop expects. Implementations are free to
// be pure functions (e.g. kernel/edict.Engine.Decide adapted by
// kernel/runtime).
type Policy func(ctx context.Context, tc ToolCall) PolicyVerdict

// DefaultMaxIter caps tool-call rounds per run (DECISIONS E5). Raised from 25 to
// 50 (M824) so deeper agentic tasks finish in one run; AGEZT_MAX_ITER overrides,
// and the chat's "Continue" resumes a run that still hits the cap.
const DefaultMaxIter = 50

// DefaultMaxAutoContinue is how many times a run is automatically continued past
// MaxIter before it gives up with ErrMaxIter (M833). With the default MaxIter of
// 50, this is up to 6×50 = 300 tool-rounds of autonomous work before a run that
// still hasn't finished stops on its own. AGEZT_MAX_AUTO_CONTINUE overrides; set
// it high (e.g. for a long unattended job) or negative to disable auto-continue.
const DefaultMaxAutoContinue = 5

// DefaultAutoContinueWait is the breather before each automatic continuation
// (M833) — short enough that chat doesn't feel hung, long enough to avoid
// hammering the provider and to leave a halt window. AGEZT_AUTO_CONTINUE_WAIT
// overrides.
const DefaultAutoContinueWait = 2 * time.Second

// autoContinuePrompt is the user turn injected when a run is auto-continued past
// MaxIter (M833). It tells the model it ran out of its round budget mid-task and
// must press on, and — crucially — to STOP and give a final answer once the work
// is actually done, so a finished task ends instead of burning continuations.
const autoContinuePrompt = "[auto-continue] You reached your tool-round budget for this segment but the task isn't finished yet. " +
	"Keep going from exactly where you left off — do not restart or repeat completed work. " +
	"As soon as the task IS fully done, stop calling tools and give your final answer."

// DefaultMaxParallelTools is the default ceiling on concurrently executing
// tool calls from one assistant turn (M880). High enough to make a typical
// fan-out (a handful of delegate/search calls) genuinely parallel, low enough
// that one turn can't stampede the host or a rate-limited upstream.
const DefaultMaxParallelTools = 4

// DefaultMaxIdenticalToolCalls is how many times the same (tool, input) may run
// in one run before the loop guard refuses further executions (M116). Generous
// enough for legitimate retries, far below MaxIter so a stuck loop can't re-run
// an expensive/failing call dozens of times.
const DefaultMaxIdenticalToolCalls = 5

// ErrMaxIter is returned by Run when MaxIter rounds elapse without a final
// assistant message.
var ErrMaxIter = errors.New("agent: max iterations exceeded")

// steeringPrefix labels an operator-injected directive (M608) so the model
// understands the new user turn is live guidance from the human operator, not a
// continuation of the original task — letting it re-prioritise accordingly.
const steeringPrefix = "[operator steering] "

// noteSteeringPrefix frames a soft "BTW" injection (M962): the model should read
// it and weave it in, but finish the current step and NOT abandon the task —
// unlike a steer, which is a re-prioritisation.
const noteSteeringPrefix = "[operator note — FYI; finish your current step, then weave this in if relevant; do NOT abandon your task] "

// ErrPanic wraps a panic recovered by Run's panic firewall (M168). It lets the
// run fail cleanly (journaled task.failed, reason=panic) instead of crashing the
// daemon goroutine. The original panic value rides in the wrapped error text.
var ErrPanic = errors.New("agent: recovered panic")

// ErrRunBudgetExceeded is returned by Run when a per-run cost cap
// (LoopConfig.MaxRunCostMicrocents) is reached (M166). Terminal, like ErrMaxIter
// — the run stops rather than the model adapting. failureReason tags it
// "cost_budget".
var ErrRunBudgetExceeded = errors.New("agent: run cost budget exceeded")

// ErrUnknownTool is returned when the model asks for a tool the loop does
// not have registered.
var ErrUnknownTool = errors.New("agent: unknown tool")

// contextSize measures the assembled context sent to the provider, by role
// (SPEC-10 §3.5 context observability). It sums each message's text content plus
// its tool-call argument JSON, grouped by role (system/user/assistant/tool), and
// returns the total and the per-role breakdown. Image attachments are excluded —
// they are a separate (vision) modality, not text context. Characters are a
// deterministic, provider-agnostic proxy for context weight (~4 chars/token); the
// goal is relative visibility into how big the context is and where it comes from
// (the basis of the context inspector and "what was in its context?"), not exact
// token billing — that lands on llm.response from the provider's real usage.
func contextSize(system string, messages []Message) (total int, byRole map[string]int) {
	byRole = make(map[string]int)
	if len(system) > 0 {
		// The system prompt is sent separately from the message list
		// (CompletionRequest.System), but it is still context that occupies the
		// window, so it counts under the "system" source.
		byRole[string(RoleSystem)] = len(system)
		total = len(system)
	}
	for _, m := range messages {
		n := len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Input)
		}
		byRole[string(m.Role)] += n
		total += n
	}
	return total, byRole
}

// maxJournaledAnswerRunes caps the answer text stored on task.completed (M51).
// The full answer is always returned to the caller; only the journaled copy —
// which lands in the append-only, hash-chained journal and is replayed on every
// projection rebuild — is bounded, so a pathologically large final message can't
// bloat the journal. The true length is preserved in the event's `chars` field.
const maxJournaledAnswerRunes = 8192

// truncateForJournal returns s unchanged when it fits the journal cap, else a
// rune-safe prefix with a marker. The byte-length fast path avoids the []rune
// allocation for the overwhelmingly common short answer (bytes ≤ cap ⇒ runes ≤
// cap, since a rune is ≥ 1 byte).
func truncateForJournal(s string) string {
	if len(s) <= maxJournaledAnswerRunes {
		return s
	}
	r := []rune(s)
	if len(r) <= maxJournaledAnswerRunes {
		return s
	}
	return string(r[:maxJournaledAnswerRunes]) + "…[truncated]"
}

// Run executes the tool-loop end-to-end:
//
//  1. Publish task.received with the user's intent.
//  2. For up to MaxIter rounds:
//     a. Publish llm.request with the current messages.
//     b. Call Provider.Complete.
//     c. Publish llm.response with the assistant's message.
//     d. If stop_reason is not tool_use → publish task.completed and
//     return Content.
//     e. Otherwise for each ToolCall: publish tool.invoked, invoke the
//     tool, publish tool.result, append a tool message.
//  3. If MaxIter elapses, return ErrMaxIter.
//
// Every step is durable-before-publish through cfg.Bus.
func Run(ctx context.Context, cfg LoopConfig, userIntent string) (answer string, runErr error) {
	if cfg.Provider == nil {
		return "", errors.New("agent: provider required")
	}
	if cfg.Bus == nil {
		return "", errors.New("agent: bus required")
	}
	if cfg.Actor == "" {
		return "", errors.New("agent: actor required")
	}
	if cfg.MaxIter <= 0 {
		cfg.MaxIter = DefaultMaxIter
	}
	if cfg.MaxIdenticalToolCalls == 0 {
		cfg.MaxIdenticalToolCalls = DefaultMaxIdenticalToolCalls
	}
	// M833: 0 → default auto-continue budget; negative → disabled (old
	// fail-at-MaxIter behaviour). Resolved once here so the loop math is simple.
	if cfg.MaxAutoContinue == 0 {
		cfg.MaxAutoContinue = DefaultMaxAutoContinue
	}
	if cfg.MaxAutoContinue < 0 {
		cfg.MaxAutoContinue = 0
	}
	if cfg.AutoContinueWait == 0 {
		cfg.AutoContinueWait = DefaultAutoContinueWait
	}
	if cfg.Tools == nil {
		cfg.Tools = map[string]Tool{}
	}

	subject := func(suffix string) string {
		// Every event in this run shares an "agent.<actor>.<suffix>"
		// subject so subscribers can scope-filter to a single agent.
		return fmt.Sprintf("agent.%s.%s", cfg.Actor, suffix)
	}

	publish := func(kind event.Kind, suffix string, payload any) (*event.Event, error) {
		return cfg.Bus.Publish(event.Spec{
			Subject:       subject(suffix),
			Kind:          kind,
			Actor:         cfg.Actor,
			CorrelationID: cfg.CorrelationID,
			Payload:       payload,
		})
	}

	// 1. task.received — records the intent and, when present, the count of
	// image attachments (M93) so the run's provenance shows it had vision input.
	received := map[string]any{"intent": userIntent}
	if len(cfg.Images) > 0 {
		received["images"] = len(cfg.Images)
	}
	// Tag the run with the named agent it executes AS (M854), so a per-agent
	// activity timeline can attribute runs — "what did researcher do?". Empty for
	// the daemon's default identity.
	if cfg.Agent != "" {
		received["agent"] = cfg.Agent
	}
	if cfg.WakeSource != "" {
		received["wake_source"] = cfg.WakeSource
	}
	if cfg.WakeReason != "" {
		received["wake_reason"] = cfg.WakeReason
	}
	if cfg.ScheduleID != "" {
		received["schedule_id"] = cfg.ScheduleID
	}
	if cfg.StandingID != "" {
		received["standing_id"] = cfg.StandingID
	}
	if cfg.StandingName != "" {
		received["standing_name"] = cfg.StandingName
	}
	if cfg.TriggerSubject != "" {
		received["trigger_subject"] = cfg.TriggerSubject
	}
	if cfg.ParentCorrelation != "" {
		received["parent_correlation"] = cfg.ParentCorrelation
	}
	if _, err := publish(event.KindTaskReceived, "task", received); err != nil {
		return "", apperrors.Wrap(ctx, "agent: publish task.received", err)
	}

	// From here the run has started: any error return is a run that began
	// but never reached task.completed. Emit a terminal task.failed exactly
	// once (best-effort) so `agt runs` can tell a real failure apart from a
	// true orphan (M28) and `agt runs stats` can split the success rate
	// (M30). A clean completion returns runErr==nil and is already terminal
	// via task.completed, so the defer no-ops. The best-effort publish must
	// not mask runErr, and must not run for the pre-task validation errors
	// above (no run started) — hence it's registered only after
	// task.received succeeds.
	defer func() {
		if runErr == nil {
			return
		}
		_, _ = publish(event.KindTaskFailed, "task", map[string]any{
			"error":  runErr.Error(),
			"reason": failureReason(ctx, runErr),
		})
	}()

	// Panic firewall (M168): the loop calls into providers and tools that may be
	// third-party out-of-process plugins. A panic in any of them would otherwise
	// unwind through this bare goroutine and crash the WHOLE daemon, killing every
	// concurrent run. Recover it into a normal error so the blast radius is this
	// one run: the panic message is captured in runErr (and thus journaled by the
	// task.failed defer above, which runs AFTER this one — defers are LIFO, and
	// this is registered last so it sets runErr first). Registered after
	// task.received so a pre-run validation panic isn't double-counted as a run.
	defer func() {
		if r := recover(); r != nil {
			runErr = fmt.Errorf("%w: %v", ErrPanic, r)
		}
	}()

	messages := []Message{
		{Role: RoleUser, Content: userIntent, Images: cfg.Images},
	}

	tools := make([]ToolDef, 0, len(cfg.Tools))
	for name, t := range cfg.Tools {
		def := t.Definition()
		if err := LintToolSchema(def); err != nil {
			return "", fmt.Errorf("agent: tool %q schema lint failed: %w", name, err)
		}
		tools = append(tools, def)
	}

	// callCounts tracks how many times each exact (tool, input) has been
	// requested in this run, for the M116 loop guard.
	callCounts := map[string]int{}

	// observations tracks the last successful output for each exact tool/input
	// pair so repeated observations can be delivered to the model as deltas.
	observations := map[string]string{}

	// toolDenials tracks how many times each tool has been refused by policy
	// this run. Once a tool reaches maxToolDenials (or is hard-denied even once)
	// it is dropped from the set offered to the model on later iterations — so
	// the model stops burning iterations (and tokens) requesting a call the
	// policy will always refuse (M605). The M116 guard only catches an identical
	// (tool,input) repeat; this catches the same tool tried with new inputs.
	toolDenials := map[string]int{}
	const maxToolDenials = 2

	// untrustedTaint carries external-observation provenance forward to policy.
	// It is set by the tool-result boundary, not by the model, so a hostile web
	// page cannot erase that it was the source of a downstream proposal.
	var untrustedTaint UntrustedObservationTaint
	// directiveObsIter is the loop iteration at which the most recent
	// directive-like untrusted observation arrived (-1 = none yet). The
	// directive flag threaded to policy is ACTIVE only while a proposed action is
	// within directiveWindow iterations of that observation — the causal window
	// in which the model could be acting on the injected instruction. After the
	// window the run keeps its audit provenance (Sources/Matches) but the gate no
	// longer fires, so one suspicious search early in a run stops forcing
	// approval on every later action (the run-wide-sticky-taint fix).
	directiveObsIter := -1
	directiveWindow := cfg.DirectiveTaintWindow
	if directiveWindow <= 0 {
		directiveWindow = DefaultDirectiveTaintWindow
	}

	// spentMicrocents accumulates this run's provider spend for the per-run cost
	// cap (M166). A local stack variable — no shared state, no lifecycle, no
	// cleanup — so the cap adds zero concurrency surface.
	var spentMicrocents int64

	// summarizeElided wraps cfg.SummarizeElided (M398) with a per-run cache keyed
	// by the output so each distinct tool output is summarised at most once, and
	// swallows ctx/errors into "" so compaction always falls back to the head
	// snippet. nil when no summarizer is configured — zero extra provider calls.
	var summarizeElided func(string) string
	if cfg.SummarizeElided != nil {
		summaryCache := map[string]string{}
		summarizeElided = func(output string) string {
			if s, ok := summaryCache[output]; ok {
				return s
			}
			s, err := cfg.SummarizeElided(ctx, output)
			if err != nil {
				s = "" // fall back to the head snippet for this output
			}
			summaryCache[output] = s
			return s
		}
	}

	// Auto-continue (M833): the loop runs in SEGMENTS of cfg.MaxIter rounds. When a
	// segment exhausts without a final answer, instead of failing immediately we
	// inject a "keep going" turn and grant another segment — up to cfg.MaxAutoContinue
	// times — so a long task finishes autonomously instead of stopping at the cap.
	// `iter` is monotonic across segments (so journal iter numbers keep climbing);
	// segmentEnd is the round budget for the current segment.
	autoContinuesUsed := 0
	segmentEnd := cfg.MaxIter
	for iter := 0; ; iter++ {
		if iter >= segmentEnd {
			// Segment exhausted without a final answer. Continue automatically if
			// budget remains; otherwise fall through to ErrMaxIter.
			if autoContinuesUsed >= cfg.MaxAutoContinue {
				break
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
			autoContinuesUsed++
			if _, err := publish(event.KindTaskContinued, "task", map[string]any{
				"attempt":      autoContinuesUsed,
				"of":           cfg.MaxAutoContinue,
				"iters_so_far": iter,
			}); err != nil {
				return "", apperrors.Wrap(ctx, "agent: publish task.continued", err)
			}
			// Breather before pressing on (ctx-aware so a halt during the wait
			// ends the run immediately rather than after the sleep).
			if cfg.AutoContinueWait > 0 {
				t := time.NewTimer(cfg.AutoContinueWait)
				select {
				case <-ctx.Done():
					t.Stop()
					return "", ctx.Err()
				case <-t.C:
				}
			}
			messages = append(messages, Message{Role: RoleUser, Content: autoContinuePrompt})
			segmentEnd += cfg.MaxIter
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Live steering (M608): honour an operator pause at this safe boundary
		// (the in-flight call + tool results of the previous iteration are already
		// settled), then fold any injected directives into the conversation as
		// fresh user turns so the model acts on them next. Wait returns ctx.Err()
		// if the run is cancelled/halted/timed-out while paused, so steering never
		// makes a run un-killable. nil Steer skips this entirely.
		if cfg.Steer != nil {
			if err := cfg.Steer.Wait(ctx); err != nil {
				return "", err
			}
			for _, d := range cfg.Steer.Drain() {
				prefix, mode := steeringPrefix, "steer"
				if d.Note {
					prefix, mode = noteSteeringPrefix, "note"
				}
				messages = append(messages, Message{Role: RoleUser, Content: prefix + d.Text})
				if _, err := publish(event.KindRunSteered, "steer", map[string]any{
					"iter":      iter,
					"directive": d.Text,
					"mode":      mode,
				}); err != nil {
					return "", apperrors.Wrap(ctx, "agent: publish run.steered", err)
				}
			}
		}

		// Context budgeting (SPEC-10 §3): before measuring/sending, trim the
		// assembled context to fit ContextBudget by eliding the oldest tool
		// outputs (system + recent turns protected). The drop is journaled so it
		// is auditable, not silent. No-op when ContextBudget is 0.
		if cfg.ContextBudget > 0 {
			before, _ := contextSize(cfg.System, messages)
			compacted, stats := compactMessagesDetailed(cfg.System, messages, cfg.ContextBudget, cfg.ContextProtectLast, cfg.ContextProtectFirst, summarizeElided, cfg.ContextRescueMarkers)
			if stats.Elided > 0 {
				messages = compacted
				after, _ := contextSize(cfg.System, messages)
				payload := map[string]any{
					"elided":               stats.Elided,
					"reclaimed_chars":      stats.Reclaimed,
					"context_chars_before": before,
					"context_chars_after":  after,
					"budget":               cfg.ContextBudget,
				}
				if stats.Rescued > 0 {
					payload["skill_rescued_count"] = stats.Rescued
					payload["skill_rescued_chars"] = stats.RescuedChars
				}
				if _, err := publish(event.KindContextCompacted, "context", payload); err != nil {
					return "", apperrors.Wrap(ctx, "agent: publish context.compacted", err)
				}
			}
		}

		// Offer only the tools the policy hasn't repeatedly refused this run
		// (M605). A no-op until a tool crosses the denial threshold, so the
		// common case allocates nothing.
		offered := tools
		if len(toolDenials) > 0 {
			filtered := make([]ToolDef, 0, len(tools))
			for _, t := range tools {
				if toolDenials[t.Name] >= maxToolDenials {
					continue
				}
				filtered = append(filtered, t)
			}
			offered = filtered
		}
		toolsBeforeDiscovery := len(offered)
		toolDiscovery := false
		if cfg.ToolSelector != nil {
			selected, err := cfg.ToolSelector(ctx, ToolSelectionRequest{
				Intent:   userIntent,
				Iter:     iter,
				Messages: messages,
				Tools:    offered,
			})
			if err != nil {
				return "", fmt.Errorf("agent: tool discovery: %w", err)
			}
			offered = normalizeSelectedTools(offered, selected)
			toolDiscovery = true
		}

		// 2a. llm.request — record what was sent: message count plus the
		// assembled context size, broken down by role (SPEC-10 §3.5 context
		// observability — the foundation of the context inspector). Lets an
		// operator see how big each call's context was and where it came from,
		// the #1 driver of cost and "lost in the middle" quality loss.
		ctxChars, ctxByRole := contextSize(cfg.System, messages)
		reqPayload := map[string]any{
			"iter":            iter,
			"messages":        len(messages),
			"model":           cfg.Model,
			"tools":           len(offered),
			"context_chars":   ctxChars,
			"context_by_role": ctxByRole,
		}
		if toolDiscovery {
			reqPayload["tool_discovery"] = true
			reqPayload["tools_before_discovery"] = toolsBeforeDiscovery
		}
		if _, err := publish(event.KindLLMRequest, "llm", reqPayload); err != nil {
			return "", apperrors.Wrap(ctx, "agent: publish llm.request", err)
		}

		req := CompletionRequest{
			Model:               cfg.Model,
			System:              cfg.System,
			Messages:            messages,
			Tools:               offered,
			MaxTokens:           cfg.MaxTokens,
			TaskType:            cfg.TaskType,   // M703: per-task model routing hint
			ModelChain:          cfg.ModelChain, // M787: per-agent model fallback chain
			Agent:               cfg.Agent,      // M793: identity for the per-agent ledger
			AgentDailyCeilingMc: cfg.AgentDailyCeilingMc,
			CorrelationID:       cfg.CorrelationID, // M47: attribute spend to this run
			JSONMode:            cfg.JSONMode,      // M314: structured-output request
			Params:              cfg.Params,        // M997: per-request sampling knobs
		}
		var resp *CompletionResponse
		// Use the streaming path when the provider advertises it
		// (M1.q.y). Each text fragment is published as an ephemeral
		// KindLLMToken event so the CLI can render tokens inline;
		// the canonical llm.response (durable) still publishes below
		// with the assembled message + final usage. Streaming
		// failures fall back to fail-the-call rather than retry via
		// Complete — the StreamingProvider contract guarantees same-
		// response semantics, so any error is a real upstream
		// problem worth surfacing.
		if sp, ok := cfg.Provider.(StreamingProvider); ok {
			tokenIter := iter
			r, err := sp.CompleteStream(ctx, req, func(c Chunk) error {
				// Reasoning delta (M317): stream a reasoning model's chain of
				// thought as an ephemeral llm.reasoning event so it's visible
				// live (agt pulse) without bloating the durable journal.
				if c.ReasoningDelta != "" {
					_, _ = cfg.Bus.PublishStreaming(event.Spec{
						Subject:       subject("llm"),
						Kind:          event.KindLLMReasoning,
						Actor:         cfg.Actor,
						CorrelationID: cfg.CorrelationID,
						Payload: map[string]any{
							"iter": tokenIter,
							"text": c.ReasoningDelta,
						},
					})
				}
				if c.TextDelta == "" {
					return nil
				}
				_, _ = cfg.Bus.PublishStreaming(event.Spec{
					Subject:       subject("llm"),
					Kind:          event.KindLLMToken,
					Actor:         cfg.Actor,
					CorrelationID: cfg.CorrelationID,
					Payload: map[string]any{
						"iter": tokenIter,
						"text": c.TextDelta,
					},
				})
				return nil
			})
			if err != nil {
				return "", apperrors.Wrapf(ctx, "agent: provider %s (stream): %w", err, cfg.Provider.Name())
			}
			resp = r
		} else {
			r, err := cfg.Provider.Complete(ctx, req)
			if err != nil {
				return "", apperrors.Wrapf(ctx, "agent: provider %s: %w", err, cfg.Provider.Name())
			}
			resp = r
			// A non-streaming provider returns the reasoning whole, with no deltas
			// (M325). Emit it as one ephemeral llm.reasoning event so a reasoning
			// model's chain of thought reaches the same consumers (agt pulse, the
			// ACP thought-chunk relay, the OpenAI-compatible API's reasoning_content)
			// that the streaming branch above already feeds live — otherwise only
			// reasoning_chars on llm.response below would record that it existed.
			if r != nil && r.ReasoningContent != "" {
				_, _ = cfg.Bus.PublishStreaming(event.Spec{
					Subject:       subject("llm"),
					Kind:          event.KindLLMReasoning,
					Actor:         cfg.Actor,
					CorrelationID: cfg.CorrelationID,
					Payload: map[string]any{
						"iter": iter,
						"text": r.ReasoningContent,
					},
				})
			}
		}
		// A provider must return a non-nil response with a nil error (the Provider
		// contract). An out-of-process plugin is third-party code that can break
		// that — e.g. (nil, nil) on an unexpected empty upstream body. Guard it:
		// every field access below assumes a non-nil resp, and a nil deref here
		// would panic the run (and, without a recover, the whole daemon).
		if resp == nil {
			return "", fmt.Errorf("agent: provider %s returned a nil response without an error", cfg.Provider.Name())
		}

		// 2c. llm.response
		if _, err := publish(event.KindLLMResponse, "llm", map[string]any{
			"iter":            iter,
			"stop_reason":     resp.StopReason,
			"usage":           resp.Usage,
			"text_chars":      len(resp.Message.Content),
			"reasoning_chars": len(resp.ReasoningContent), // M317: reasoning size (content streamed separately)
			"tool_calls":      len(resp.Message.ToolCalls),
		}); err != nil {
			return "", apperrors.Wrap(ctx, "agent: publish llm.response", err)
		}

		// Per-run cost cap (M166): add this call's spend and stop if the run has
		// reached its cap. Like the daily ceiling, the check is post-call, so a run
		// can overshoot by at most the call that crosses the line — bounded and
		// predictable. The model the call billed under is the response's reported
		// model, falling back to the requested one (same rule as the Governor).
		if cfg.CostFn != nil && cfg.MaxRunCostMicrocents > 0 {
			billed := resp.Usage.Model
			if billed == "" {
				billed = cfg.Model
			}
			spentMicrocents += cfg.CostFn(billed, resp.Usage.InputTokens, resp.Usage.OutputTokens)
			if spentMicrocents >= cfg.MaxRunCostMicrocents {
				return "", fmt.Errorf("%w (spent ~%d, cap %d microcents)", ErrRunBudgetExceeded, spentMicrocents, cfg.MaxRunCostMicrocents)
			}
		}

		messages = append(messages, resp.Message)

		// 2d. final answer?
		if resp.StopReason != StopToolUse || len(resp.Message.ToolCalls) == 0 {
			// Journal the answer text (M51) so `agt runs show` can display what
			// the run produced — the renderers expected it but it was never
			// emitted. The bus redactor scrubs secrets from the payload before it
			// lands in the journal (M15); the stored copy is length-capped so a
			// pathological output can't bloat the hash-chained, replayed journal.
			// The FULL answer is still returned to the caller unchanged.
			if _, err := publish(event.KindTaskCompleted, "task", map[string]any{
				"iters":   iter + 1,
				"chars":   len(resp.Message.Content),
				"stopped": resp.StopReason,
				"answer":  truncateForJournal(resp.Message.Content),
			}); err != nil {
				return "", apperrors.Wrap(ctx, "agent: publish task.completed", err)
			}
			return resp.Message.Content, nil
		}

		// 2e. tool calls — three phases (M880). Phase 1 gates each call
		// sequentially in call order (loop guard, policy) and journals
		// policy.decision / tool.invoked, so the audit trail and any HITL
		// approval prompts stay deterministic. Phase 2 executes the allowed
		// invocations, concurrently when the turn carries more than one (up
		// to MaxParallelTools). Phase 3 journals tool.result and appends the
		// tool messages in the ORIGINAL call order, so the conversation the
		// model sees is identical to the sequential build.
		type toolJob struct {
			tc           ToolCall
			tool         Tool // non-nil ⇒ phase 2 executes it
			result       Result
			invokeErr    error
			toolTimedOut bool
			panicked     bool
			memoEligible bool
			memoHit      bool
			memoSource   *toolJob
		}
		jobs := make([]*toolJob, 0, len(resp.Message.ToolCalls))
		memoPending := map[string]*toolJob{}
		for _, tc := range resp.Message.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			job := &toolJob{tc: tc}
			jobs = append(jobs, job)

			tool, ok := cfg.Tools[tc.Name]
			if !ok {
				job.result = Result{
					Output:  fmt.Sprintf("tool %q is not available", tc.Name),
					IsError: true,
				}
				continue
			}
			def := tool.Definition()
			if err := ValidateToolInput(def, tc.Input); err != nil {
				job.result = Result{
					Output:  "tool call rejected by schema: " + err.Error(),
					IsError: true,
				}
				continue
			}

			// Loop guard (M116): if the model has already invoked this EXACT
			// (tool, input) the cap number of times in this run, refuse to run it
			// again — re-executing a stuck/failing call produces the same result
			// and wastes work (and money). Feed back a clear nudge so the model
			// changes approach instead of looping to MaxIter. Identical-input
			// only, so a legitimate re-call with different args is unaffected. A
			// negative cap disables the guard.
			if cfg.MaxIdenticalToolCalls > 0 {
				callKey := tc.Name + "\x00" + string(tc.Input)
				callCounts[callKey]++
				if callCounts[callKey] > cfg.MaxIdenticalToolCalls {
					job.result = Result{
						Output:  fmt.Sprintf("loop guard: %q was already called with this exact input %d times in this run; the result will not change. Use different input or stop and give your answer.", tc.Name, cfg.MaxIdenticalToolCalls),
						IsError: true,
					}
					continue
				}
			}

			// Policy gate (Edict). Publishes a policy.decision event even
			// when no Policy is configured, so the journal makes the
			// gating posture explicit. A Deny verdict short-circuits the
			// invocation and synthesises a tool result the model sees.
			verdict := PolicyVerdict{Allow: true, Capability: tc.Name, Reason: "no policy configured"}
			if cfg.Policy != nil {
				policyCtx := WithPolicyToolDef(ctx, def)
				// Thread the taint with DirectiveLike scoped to the causal window:
				// active only while this action is within directiveWindow iterations
				// of the directive-like observation. Provenance (Sources/Matches)
				// is always carried for audit; only the gating flag decays.
				scopedTaint := untrustedTaint
				scopedTaint.DirectiveLike = directiveObsIter >= 0 && iter-directiveObsIter <= directiveWindow
				policyCtx = WithUntrustedObservationTaint(policyCtx, scopedTaint)
				verdict = cfg.Policy(policyCtx, tc)
			}
			if _, err := publish(event.KindPolicyDecision, "policy", map[string]any{
				"tool":                  tc.Name,
				"call_id":               tc.ID,
				"capability":            verdict.Capability,
				"allow":                 verdict.Allow,
				"reason":                verdict.Reason,
				"would_ask":             verdict.WouldAsk,
				"hard_denied":           verdict.HardDenied,
				"effect_class":          verdict.EffectClass,
				"affected_resources":    verdict.AffectedResources,
				"epistemic_action":      verdict.EpistemicAction,
				"epistemic_reason":      verdict.EpistemicReason,
				"epistemic_signals":     verdict.EpistemicSignals,
				"epistemic_confidence":  verdict.EpistemicConfidence,
				"failure_matches":       verdict.FailureMatches,
				"weighted_failures":     verdict.WeightedFailures,
				"schema_hash":           verdict.SchemaHash,
				"input_shape":           verdict.InputShape,
				"temporal_sensitive":    verdict.TemporalSensitive,
				"novel_tool":            verdict.NovelTool,
				"untrusted_observation": verdict.UntrustedObservation,
				"observation_sources":   verdict.ObservationSources,
				"directive_like":        verdict.ObservationDirectiveLike,
				"directive_matches":     verdict.ObservationDirectiveMatches,
			}); err != nil {
				return "", apperrors.Wrap(ctx, "agent: publish policy.decision", err)
			}

			if !verdict.Allow {
				// Count the refusal so a tool that's hard-denied (never allowed)
				// or repeatedly refused is dropped from later iterations (M605).
				if verdict.HardDenied {
					toolDenials[tc.Name] = maxToolDenials
				} else {
					toolDenials[tc.Name]++
				}
				job.result = Result{
					Output:  "tool call denied by policy: " + verdict.Reason,
					IsError: true,
				}
				continue // skip tool.invoked when the call never runs
			}
			if cfg.ToolMemo != nil && (verdict.EffectClass == string(EffectReadOnly) || def.Effect.Class == EffectReadOnly) {
				if cached, ok := cfg.ToolMemo.Get(tc.Name, tc.Input); ok {
					job.result = cached
					job.memoHit = true
					continue
				}
				job.memoEligible = true
				key := memoKey(tc.Name, tc.Input)
				if source, ok := memoPending[key]; ok {
					job.memoSource = source
					job.memoHit = true
					continue
				}
				memoPending[key] = job
			}
			if _, err := publish(event.KindToolInvoked, "tool", map[string]any{
				"tool":    tc.Name,
				"call_id": tc.ID,
				"input":   tc.Input,
			}); err != nil {
				return "", apperrors.Wrap(ctx, "agent: publish tool.invoked", err)
			}
			job.tool = tool
		}

		// Phase 2: execute. invoke runs ONE job; identical semantics to the
		// historical inline invocation, including the per-tool wall-clock
		// (M34): bound this single invocation without bounding the whole run.
		// Whether the tool's OWN per-call deadline fired is captured BEFORE
		// cancelling — calling toolCancel() would flip a not-yet-expired
		// toolCtx to Canceled and mask the distinction. We key on toolCtx's
		// state rather than the returned error so a tool that wraps its error
		// without the DeadlineExceeded sentinel (e.g. the warden's "context
		// deadline exceeded" string) is still classified cleanly.
		invoke := func(job *toolJob) {
			toolCtx := WithCorrelation(ctx, cfg.CorrelationID)
			var toolCancel context.CancelFunc
			if cfg.ToolTimeout > 0 {
				toolCtx, toolCancel = context.WithTimeout(toolCtx, cfg.ToolTimeout)
			}
			job.result, job.invokeErr = job.tool.Invoke(toolCtx, job.tc.Input)
			job.toolTimedOut = cfg.ToolTimeout > 0 && toolCtx.Err() == context.DeadlineExceeded
			if toolCancel != nil {
				toolCancel()
			}
		}
		var toExec []*toolJob
		for _, job := range jobs {
			if job.tool != nil {
				toExec = append(toExec, job)
			}
		}
		maxPar := cfg.MaxParallelTools
		if maxPar == 0 {
			maxPar = DefaultMaxParallelTools
		}
		if len(toExec) <= 1 || maxPar <= 1 {
			// Single call (the common case) or parallelism disabled: invoke
			// inline on this goroutine — Run's panic firewall (M168) covers it.
			for _, job := range toExec {
				invoke(job)
			}
		} else {
			// Concurrent fan-out. Each worker needs its own panic recovery:
			// Run's firewall only guards Run's goroutine, and an unrecovered
			// panic on a spawned goroutine would crash the whole daemon. A
			// captured panic is surfaced in phase 3 as the run's terminal
			// error — exactly what the sequential path would have produced.
			sem := make(chan struct{}, maxPar)
			var wg sync.WaitGroup
			for _, job := range toExec {
				wg.Add(1)
				go func(job *toolJob) {
					defer wg.Done()
					defer func() {
						if r := recover(); r != nil {
							job.panicked = true
							job.invokeErr = fmt.Errorf("%w: %v", ErrPanic, r)
						}
					}()
					sem <- struct{}{}
					defer func() { <-sem }()
					invoke(job)
				}(job)
			}
			wg.Wait()
		}

		// Phase 3: classify, journal, and append in the original call order.
		for _, job := range jobs {
			if job.memoSource != nil {
				job.result = job.memoSource.result
			}
			if job.tool != nil {
				switch {
				case job.panicked:
					return "", job.invokeErr
				case job.invokeErr == nil:
					// job.result already holds the tool's output.
				case ctx.Err() != nil:
					// The RUN context itself ended (operator halt, M32
					// cancel, or the M31 per-run deadline) — a run-level
					// terminal, not a tool fault. Propagate so the run
					// fails with the correct reason instead of limping on.
					return "", ctx.Err()
				case job.toolTimedOut:
					// The tool overran its own budget while the run is fine:
					// hand the model a clear error and keep the run going.
					job.result = Result{
						Output:  fmt.Sprintf("tool %q exceeded its %s timeout", job.tc.Name, cfg.ToolTimeout),
						IsError: true,
					}
				default:
					job.result = Result{Output: job.invokeErr.Error(), IsError: true}
				}
				if job.memoEligible && !job.result.IsError {
					cfg.ToolMemo.Set(job.tc.Name, job.tc.Input, job.result)
				}
			}

			modelOutput := job.result.Output
			observationDelta := false
			if cfg.ObservationDeltas && !job.result.IsError {
				key := job.tc.Name + "\x00" + string(job.tc.Input)
				if prev, ok := observations[key]; ok {
					if delta, changed := DiffObservation(prev, job.result.Output); changed {
						modelOutput = delta
						observationDelta = true
					}
				}
				observations[key] = job.result.Output
			}

			observationBoundary := ObservationBoundaryForTool(job.tc.Name, job.result, modelOutput)
			if observationBoundary.Trust == ObservationUntrusted {
				untrustedTaint = MergeUntrustedObservationTaint(untrustedTaint, observationBoundary)
				if observationBoundary.DirectiveLike {
					// Remember WHEN the directive-like observation arrived so the
					// gate can decay after directiveWindow iterations.
					directiveObsIter = iter
				}
				modelOutput = RenderObservationForModel(job.tc.Name, observationBoundary, modelOutput)
			}

			// Offload a large output out of the journal event (SPEC-04 §3.6 /
			// SPEC-01 §10.2): the event carries a preview + raw_ref. The model
			// gets the raw output by default, or the observation delta when the
			// optional CH-04 delta layer is enabled for a repeated observation.
			eventOutput, rawRef, fullBytes, offloaded := offloadToolOutput(cfg.Artifacts, cfg.ArtifactThreshold, job.result.Output)
			resultPayload := map[string]any{
				"tool":               job.tc.Name,
				"call_id":            job.tc.ID,
				"output":             eventOutput,
				"error":              job.result.IsError,
				"observation_trust":  observationBoundary.Trust,
				"observation_source": observationBoundary.Source,
				"directive_like":     observationBoundary.DirectiveLike,
				"directive_matches":  observationBoundary.Matches,
			}
			if offloaded {
				resultPayload["raw_ref"] = rawRef
				resultPayload["output_bytes"] = fullBytes
			}
			if observationDelta {
				resultPayload["observation_delta"] = true
				resultPayload["model_output_bytes"] = len(modelOutput)
				resultPayload["raw_output_bytes"] = len(job.result.Output)
			}
			if job.memoHit {
				resultPayload["memo_hit"] = true
			}
			if _, err := publish(event.KindToolResult, "tool", resultPayload); err != nil {
				return "", apperrors.Wrap(ctx, "agent: publish tool.result", err)
			}
			if cfg.ToolResultHook != nil && job.tool != nil {
				cfg.ToolResultHook(ctx, job.tc, job.result)
			}

			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    modelOutput,
				ToolCallID: job.tc.ID,
			})
		}
	}

	return "", ErrMaxIter
}

// failureReason classifies a run's terminal error into a short, stable
// tag carried on the task.failed payload (the full error string rides
// alongside in "error"). Operators and `agt runs` use the tag to group
// failures without parsing free-form error text:
//
//	panic       — a provider/tool panicked; the firewall recovered it (M168).
//	max_iters   — the loop exhausted MaxIter without a final answer.
//	cost_budget — the per-run cost cap (MaxRunCostMicrocents) was reached (M166).
//	canceled    — the context was cancelled (operator halt / shutdown).
//	timeout     — the context deadline elapsed (a per-run / daemon timeout).
//	error       — anything else (provider error, publish failure, …).
//
// errors.Is is used so a wrapped provider error that ultimately carries
// context.Canceled/DeadlineExceeded is still classified correctly; the
// ctx.Err() fallback covers the bare-return cancellation paths.
func failureReason(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, ErrPanic):
		return "panic"
	case errors.Is(err, ErrMaxIter):
		return "max_iters"
	case errors.Is(err, ErrRunBudgetExceeded):
		return "cost_budget"
	case errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled:
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded:
		return "timeout"
	default:
		return "error"
	}
}
