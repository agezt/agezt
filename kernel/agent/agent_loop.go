// SPDX-License-Identifier: MIT

// Agent loop config: LoopConfig.
// Code extracted from agent.go during the Day-78 god-file split. Public API unchanged.
package agent


import (
	"context"
	"github.com/agezt/agezt/kernel/bus"
	"time"
)


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