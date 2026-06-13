// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"time"
)

// IterationConfig controls iteration limits and tool execution bounds.
// These fields govern how many rounds the loop runs and how long each may take.
type IterationConfig struct {
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
}

// ModelConfig configures the LLM provider and model selection for a run.
type ModelConfig struct {
	// Model is the model to use (e.g. "claude-opus-4-5").
	Model string
	// System is the system prompt.
	System string
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
	// MaxTokens passed to the provider per call. 0 → provider default.
	MaxTokens int
	// JSONMode requests structured (JSON) output on every provider call of the
	// run (M314). Set by callers that need a machine-parseable result; a provider
	// without a native JSON mode ignores it. Flows to CompletionRequest.JSONMode.
	JSONMode bool
}

// AgentIdentity carries the identity and correlation for a run.
// These fields identify WHO is running and WHY this particular run exists.
type AgentIdentity struct {
	// Agent is the roster agent slug this run executes AS (M793).
	// Empty for ad-hoc chat runs.
	Agent string
	// AgentDailyCeilingMc is the per-day spend ceiling for this named agent (M793).
	// 0 means no ceiling.
	AgentDailyCeilingMc int64
	// Actor is the journaling actor for emitted events (e.g. "agent-01H").
	Actor string
	// CorrelationID ties every event in this run together.
	CorrelationID string
}

// BudgetConfig controls cost accounting and spend limits for a run.
type BudgetConfig struct {
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
}

// ContextConfig controls context window management and compaction (SPEC-10 §3).
type ContextConfig struct {
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
}

// ArtifactConfig controls large output offloading to content-addressed storage.
type ArtifactConfig struct {
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
}

// NewIterationConfig returns a default IterationConfig.
func NewIterationConfig() IterationConfig {
	return IterationConfig{
		MaxIter:            25, // DECISIONS E5 default
		MaxAutoContinue:    0,  // use DefaultMaxAutoContinue
		AutoContinueWait:   0,  // use DefaultAutoContinueWait
		ToolTimeout:        0,  // unbounded
		MaxParallelTools:   0,  // use DefaultMaxParallelTools
		MaxIdenticalToolCalls: 0, // use DefaultMaxIdenticalToolCalls
	}
}

// NewModelConfig returns a default ModelConfig.
func NewModelConfig() ModelConfig {
	return ModelConfig{
		MaxTokens: 0, // provider default
	}
}

// NewAgentIdentity returns a default AgentIdentity.
func NewAgentIdentity() AgentIdentity {
	return AgentIdentity{}
}

// NewBudgetConfig returns a default BudgetConfig (no limits).
func NewBudgetConfig() BudgetConfig {
	return BudgetConfig{} // zero values = no limits
}

// NewContextConfig returns a default ContextConfig (compaction disabled).
func NewContextConfig() ContextConfig {
	return ContextConfig{} // zero values = disabled
}

// NewArtifactConfig returns a default ArtifactConfig (offload disabled).
func NewArtifactConfig() ArtifactConfig {
	return ArtifactConfig{} // nil Artifacts = disabled
}
