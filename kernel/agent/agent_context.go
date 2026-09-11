// SPDX-License-Identifier: MIT

package agent

// Agent context budget + compaction + offload helpers
// (AutoContextBudgetChars + headSnippet + compactMessagesDetailed +
// rescuedToolOutput + offloadToolOutput + contextSize + truncateForJournal).
// Carved out of agent.go during the Day 31 god file split #1 so the
// main file can focus on Run + ToolCapability.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

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
