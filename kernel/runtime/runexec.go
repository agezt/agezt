// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/tenantctx"
)

// This file is the kernel's run engine: the entry points an external
// caller drives (Run / RunAssured / RunWithRetry / RunWith), the
// post-run hook trio (maybeShadowEval / maybeForge / maybeDistill), the
// deterministic heuristic bypass, and the journal re-exports
// (Why / Causes / ParentOf / Verify). Pulled out of runtime.go as the
// fourth step of the Day 9 split (after lifecycle.go, compose.go,
// accessors.go).
//
// The single piece that requires extra care here is the lock-ordering
// invariant documented on Kernel:
//
//   configMu (light config) < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu
//
// The deferred cleanup at the bottom of RunWith takes all five
// (runsMu → fanoutMu → treeMu → steersMu → spawnsMu) in this exact
// order and releases them in reverse. Any new locking site added in
// this file MUST respect the same order, or `go test -race` will trip.
// The Live-steering registration right after the runsMu unlock also
// follows the rule (steersMu acquired AFTER runsMu is released — see
// the comment at runtime.go:910-919 for the full history).

// Run executes one tool-loop end-to-end and returns (answer, corr, err).
// It mints a correlation ID internally; for the subscribe-then-run flow
// the control plane uses, see NewCorrelation + RunWith.
func (k *Kernel) Run(ctx context.Context, intent string) (string, string, error) {
	corr := k.NewCorrelation()
	ans, err := k.RunWith(ctx, corr, intent)
	return ans, corr, err
}

// assureVerifyMaxTokens bounds the verifier completion — it only emits a tiny
// JSON verdict, so a small cap keeps the completion check cheap.
const assureVerifyMaxTokens = 400

// RunAssured is the "do-it-for-sure" loop (M651): it runs the intent, asks a
// verifier whether the task was actually accomplished, and retries with the gap
// fed back — up to maxAttempts, stopping the moment the task is judged complete.
// Every attempt reuses corr (they run sequentially and never overlap), so the
// whole objective streams and journals under one correlation id. Returns the
// final answer and the loop result (attempts, completion, per-attempt history).
// Day 33: the body lives in runexec.Runner.RunAssured; this wrapper preserves
// the legacy *Kernel.RunAssured public surface.
func (k *Kernel) RunAssured(ctx context.Context, corr, intent string, maxAttempts int) (string, assure.Result, error) {
	return k.runexec.RunAssured(ctx, corr, intent, maxAttempts)
}

// RunWithRetry executes one agent run using the profile's failure retry policy.
// This is distinct from provider retry (one LLM request) and RunAssured
// (semantic completion verification): it retries the whole governed run after a
// terminal error, journaling each retry decision under the same correlation.
//
// Day 23: the body lives in the runexec sub-package's Runner.
// The legacy method body remains below for reference and the
// file's lock-ordering invariant documentation; the *Kernel
// surface delegates to the Runner for callers that already
// hold a *Kernel.
func (k *Kernel) RunWithRetry(ctx context.Context, corr, intent string, pol roster.RetryPolicy) (string, error) {
	return k.runexec.RunWithRetry(ctx, corr, intent, pol)
}

// ClaimResumeTicket is the public surface for the resume-ticket
// ownership dance. Returns the (possibly ticket-bearing) ctx and
// whether THIS frame owns the ticket (true = caller must finalize).
// Wraps claimResumeTicket in resume.go so the runexec sub-package
// can drive it through KernelAPI (Day 23).
func (k *Kernel) ClaimResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool) {
	return k.claimResumeTicket(ctx, corr, intent, kind, assureBudget)
}

// FinalizeResumeTicket is the public surface for clearing or
// keeping a resume ticket on run exit. Honours k.suspending so
// shutdown-interrupted runs keep their ticket. Wraps
// finalizeResumeTicket in resume.go (Day 23).
func (k *Kernel) FinalizeResumeTicket(corr string, runErr error) {
	k.finalizeResumeTicket(corr, runErr)
}

// RetryReason classifies a Run/RunWith terminal error for the
// agent-retry event payload. Used by Runner.RunWithRetry and
// subagent.go's retry loop; both reach the implementation
// through the public method (the package-level helper stays
// available for the in-package subagent.go call site).
func (k *Kernel) RetryReason(err error) string { return retryReason(err) }

// AgentRetryable reports whether the given reason matches the
// policy's RetryOn allowlist. Empty RetryOn = default ("error"
// and "timeout" only — never auto-retry a halted/cancelled run).
func (k *Kernel) AgentRetryable(reason string, retryOn []string) bool { return agentRetryable(reason, retryOn) }

// RetryDelay computes the wait between attempts. Exponential
// backoff doubles each attempt; MaxDelaySec caps the result.
// Zero base delay = no wait (try immediately).
func (k *Kernel) RetryDelay(pol roster.RetryPolicy, attempt int) time.Duration { return retryDelay(pol, attempt) }

func retryReason(err error) string {
	switch {
	case errors.Is(err, ErrHalted):
		return "halted"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "error"
	}
}

func agentRetryable(reason string, retryOn []string) bool {
	if len(retryOn) == 0 {
		return reason == "error" || reason == "timeout"
	}
	for _, r := range retryOn {
		if strings.TrimSpace(r) == reason {
			return true
		}
	}
	return false
}

func retryDelay(pol roster.RetryPolicy, attempt int) time.Duration {
	base := time.Duration(pol.BaseDelaySec) * time.Second
	if base <= 0 {
		return 0
	}
	delay := base
	if strings.TrimSpace(pol.Backoff) == "exponential" {
		for i := 1; i < attempt; i++ {
			delay *= 2
		}
	}
	if pol.MaxDelaySec > 0 {
		max := time.Duration(pol.MaxDelaySec) * time.Second
		if delay > max {
			delay = max
		}
	}
	return delay
}

// verifyCompletion body moved to runexec.Runner.VerifyCompletion on
// Day 33. The public wrapper (runtime.go:VerifyCompletion) delegates
// to the Runner.

// visionDescribeMaxTokens / assureVerifyMaxTokens / shadowEvalLimit
// moved to runexec/helpers.go on Day 33.

// ErrNoVisionModel is the run-engine sentinel returned by
// DescribeImages when no vision-capable model is available.
// The canonical definition lives in the runtime package
// (referenced by cmd/agezt/main.go:1909); kept here as a
// package-level value (separate identity, same text) so
// callers inside runexec can use it. The *Kernel.DescribeImages
// wrapper translates via errors.Is so external callers see
// the canonical runtime.ErrNoVisionModel.
var ErrNoVisionModel = errors.New("runtime: no vision-capable model available")

// DescribeImages body moved to runexec.Runner.DescribeImages on
// Day 33. Callers in this package reach it through *Kernel.DescribeImages
// (see compose.go / research.go for the production callers).

// RunWith executes one tool-loop using the supplied correlation ID.
// If the kernel is halted before this Run starts, returns ErrHalted. If
// Halt is called during the Run, ctx is cancelled and RunWith returns
// context.Canceled.
//
// Day 34: the body moved to runexec.Runner.RunWith. The lock-
// protected bits (setupRunState / cleanupRunState /
// deregisterRunSteer) live as private methods on *Kernel and are
// reached through SetupRunState / CleanupRunState /
// DeregisterRunSteer KernelAPI wrappers; *Kernel.RunWith just
// delegates so callers compile unchanged.
func (k *Kernel) RunWith(ctx context.Context, corr, intent string) (string, error) {
	if corr == "" {
		return "", errors.New("runtime: correlation id required")
	}
	return k.runexec.RunWith(ctx, corr, intent)
}

// completeAgentLifecycle / shouldRetireAgentAfterComplete /
// resetCompletedCycleTasks moved to the runexec sub-package on
// Day 33 (Runner.CompleteAgentLifecycle + helpers.go). The
// *Kernel.CompleteAgentLifecycle public wrapper above delegates
// to the Runner. RunWith's call site uses that wrapper.

// CompleteAgentLifecycle advances the durable lifecycle for a successful
// non-loop job that ran under an agent profile, such as a scheduled workflow or
// direct tool target. RunWith calls the Runner's CompleteAgentLifecycle
// itself; external runners should call this only after the job has
// genuinely succeeded. Day 33: the body lives in
// runexec.Runner.CompleteAgentLifecycle; the wrapper preserves the
// legacy public surface.
func (k *Kernel) CompleteAgentLifecycle(ctx context.Context, corr string) {
	k.runexec.CompleteAgentLifecycle(ctx, corr)
}

// setupRunState is the private body behind the public
// SetupRunState wrapper (Day 34). Returns the decorated runCtx,
// the cancel func, the steer interface, and an error. Lock
// order runsMu → steersMu; live-steering slot acquisition
// sits AFTER runsMu is released (see runtime.go:~640-650 for
// the history of this invariant).
func (k *Kernel) setupRunState(corr string, parentCtx context.Context) (context.Context, context.CancelFunc, agent.Steerer, error) {
	ctx := parentCtx
	k.runsMu.Lock()
	if k.halted {
		k.runsMu.Unlock()
		return nil, nil, nil, ErrHalted
	}
	// Reject a correlation that is already running: two concurrent
	// RunWith calls sharing one id would clobber the run registry —
	// the second's cancel overwrites the first's k.runs[corr], and
	// the first's deferred delete then removes the second's entry,
	// leaving a run uncancellable by Halt/CancelRun. The contract is
	// one id per run; enforce it instead of silently corrupting the
	// registry. (M480)
	if _, running := k.runs[corr]; running {
		k.runsMu.Unlock()
		return nil, nil, nil, fmt.Errorf("runtime: correlation %q is already running", corr)
	}
	// Per-run wall-clock budget (M31): when configured, the run
	// context carries a deadline so a slow provider / blocking tool
	// can't hang a run forever within a live session. The deadline
	// cancels with DeadlineExceeded (→ task.failed reason=timeout,
	// M30), whereas the cancel stored in k.runs (invoked by Halt)
	// cancels with Canceled (→ reason=canceled) — the two stay
	// distinguishable. 0 = no cap. A per-run override (WithRunTimeout,
	// e.g. `agt run --timeout`) takes precedence over the daemon-wide
	// MaxDuration; either yields a deadline that cancels with
	// DeadlineExceeded.
	// Stamp the tenant identity onto the run context so tenant-aware
	// tools can read it (M219). No-op for the primary kernel (empty
	// TenantID). Done before deriving runCtx so the value propagates
	// through the timeout/cancel context to every tool call.
	ctx = tenantctx.WithTenant(ctx, k.cfg.TenantID)
	if len(k.cfg.AutoApproveCapabilities) > 0 {
		ctx = WithAutoApproveCapabilities(ctx, mergeAutoApproveCapabilities(ctx, k.cfg.AutoApproveCapabilities))
	}

	maxDur := k.cfg.MaxDuration
	if d := runTimeoutFromCtx(ctx); d > 0 {
		maxDur = d
	}
	var runCtx context.Context
	var cancel context.CancelFunc
	if maxDur > 0 {
		runCtx, cancel = context.WithTimeout(ctx, maxDur)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	k.runs[corr] = cancel
	// Drain accounting (M883): Close waits (bounded) for in-flight
	// runs to settle before tearing down the stores they write to.
	// Add under the same lock as the halted check above, so no run
	// can slip in after Halt flips the flag and Close begins waiting.
	k.runWG.Add(1)
	// Live-steering control surface (M608): registered for the run's
	// whole lifetime so an operator can pause/step/inject from
	// another goroutine. Wired into the agent loop via
	// LoopConfig.Steer below. k.steers is guarded by steersMu
	// everywhere else (controlFor, the deregistration paths,
	// resume's shutdown broadcast) — taking only runsMu here raced
	// with them (caught by -race under concurrent Runs). Lock order
	// runsMu → steersMu is respected.
	rc := newRunControl()
	k.steersMu.Lock()
	k.steers[corr] = rc
	k.steersMu.Unlock()
	k.runsMu.Unlock()
	return runCtx, cancel, rc, nil
}

// cleanupRunState is the private body behind the public
// CleanupRunState wrapper. Encapsulates the 5-mutex lock-ordering
// invariant: runsMu → fanoutMu → treeMu → steersMu → spawnsMu.
// Returns the orphan cancel funcs (child spawns that were still in
// flight); caller invokes them, then invokes the runCtx's cancel.
// Day 34: also calls runWG.Done() to balance the Add(1) in
// setupRunState — without it Close() would deadlock on its
// bounded Wait (M883 drain accounting).
func (k *Kernel) cleanupRunState(corr string) []context.CancelFunc {
	// Drain accounting (M883): Close waits (bounded) for in-flight
	// runs to settle before tearing down the stores they write to.
	// Add() lives in setupRunState so it pairs with the halted +
	// duplicate-corr guard; Done() lives here so the waitgroup
	// decrement is the LAST step a run takes (after every map /
	// mutex / spawn-cancel cleanup), and Close's WaitGroup.Wait
	// sees a counter that monotonically tracks the active set.
	defer k.runWG.Done()
	// Lock ordering: runsMu → fanoutMu → treeMu → steersMu → spawnsMu
	// Lock ordering: runsMu → fanoutMu → treeMu → steersMu → spawnsMu
	k.runsMu.Lock()
	delete(k.runs, corr)
	k.fanoutMu.Lock()
	delete(k.fanout, corr) // release this run's fan-out tally (M46)
	k.treeMu.Lock()
	delete(k.tree, corr) // release this tree's total sub-agent tally (M629)
	k.steersMu.Lock()
	delete(k.steers, corr) // release the steering control (M608)
	k.spawnsMu.Lock()
	// Cancel any still-pending async delegations of this tree (M881):
	// an un-awaited child must not outlive the run that spawned it.
	// The spawn goroutine observes the cancel, finishes, and journals
	// its terminal events; the handle is dropped here so the id is no
	// longer awaitable.
	var orphans []context.CancelFunc
	for id, h := range k.spawns {
		if h.rootCorr == corr || h.parentCorr == corr {
			orphans = append(orphans, h.cancel)
			delete(k.spawns, id)
		}
	}
	k.spawnsMu.Unlock()
	k.steersMu.Unlock()
	k.treeMu.Unlock()
	k.fanoutMu.Unlock()
	k.runsMu.Unlock()
	return orphans
}

// deregisterRunSteer removes the steer handle for corr without
// acquiring runsMu. Used post-run to free the steering slot the
// instant the agent loop returns — BEFORE the deferred
// cleanupRunState runs (which would also delete it, but later in
// the post-processing pipeline; an operator pausing/steering in
// that window would otherwise get a false success against a loop
// that has already finished, M608). delete is idempotent.
func (k *Kernel) deregisterRunSteer(corr string) {
	k.steersMu.Lock()
	delete(k.steers, corr)
	k.steersMu.Unlock()
}

// publishHeuristicBypass body moved to runexec.Runner.PublishHeuristicBypass
// on Day 33. RunWith calls it through k.runexec.PublishHeuristicBypass.

// deterministicHeuristicBypass moved to runexec/helpers.go on Day 34
// so the Runner can use it through the package boundary.

// maybeDistill / maybeForge / maybeShadowEval moved to the runexec
// sub-package on Day 32 — Runner.MaybeDistill / MaybeForge /
// MaybeShadowEval hold the bodies. The *Kernel wrappers in
// runtime.go delegate to the Runner.

// elidedSummaryMaxTokens bounds the abstractive summary call (M398): one short
// line, so a small cap keeps the extra spend negligible and the latency low.
const elidedSummaryMaxTokens = 64

// elidedSummaryReasoningMaxTokens is the cap when the run's model is a
// reasoning model (M926): such models spend output tokens on their chain of
// thought BEFORE the summary line, so the tight cap gets entirely consumed and
// Complete returns empty content — observed live on deepseek-v4-pro at 64
// (every abstractive summary silently degraded to the extractive head stub).
// The prompt still asks for one line; the headroom is only used by models that
// actually reason.
const elidedSummaryReasoningMaxTokens = 1024

// elidedSummaryInputCap bounds how much of a dropped output is fed to the
// summarizer — enough to summarise, while keeping the summary call's own input
// (and therefore its cost) bounded regardless of how large the output was.
const elidedSummaryInputCap = 8 << 10

// makeElidedSummarizer builds the LoopConfig.SummarizeElided closure: a bounded,
// single-shot provider call that condenses a dropped tool output to one line
// (M398). It routes through the same provider (the Governor) as the run, so the
// extra call is billed and attributed to the run via corr. Errors propagate; the
// loop swallows them and falls back to the deterministic head snippet.
// maxTokens is caller-chosen: tight for plain models, roomy for reasoning
// models whose chain of thought eats the budget first (M926).
func makeElidedSummarizer(provider agent.Provider, model, corr string, maxTokens int) func(context.Context, string) (string, error) {
	return func(ctx context.Context, output string) (string, error) {
		in := output
		if len(in) > elidedSummaryInputCap {
			in = in[:elidedSummaryInputCap]
		}
		resp, err := provider.Complete(ctx, agent.CompletionRequest{
			Model:         model,
			CorrelationID: corr,
			TaskType:      "summarize",
			MaxTokens:     maxTokens,
			Messages: []agent.Message{{
				Role:    agent.RoleUser,
				Content: "Summarize this tool output in one short line for an agent's working memory. Output only the summary.\n\n" + in,
			}},
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
}

// FoldRunTools counts tool.result events for corr and collects the
// tool names invoked (in order), for the distillation transcript.
// Public wrapper for the runexec sub-package (Day 32). The body
// stays on *Kernel because the journal range is the per-process
// append-only log; the runner reads through this handle.
func (k *Kernel) FoldRunTools(corr string) (int, []string) {
	var (
		count int
		names []string
	)
	_ = k.journal.Range(func(e *event.Event) error {
		if e.CorrelationID != corr || e.Kind != event.KindToolResult {
			return nil
		}
		count++
		var p struct {
			Tool string `json:"tool"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Tool != "" {
			names = append(names, p.Tool)
		}
		return nil
	})
	return count, names
}

// buildTranscript moved to the runexec sub-package on Day 32
// (runexec/helpers.go). The Runner uses it for MaybeDistill /
// MaybeForge / MaybeShadowEval transcripts.

// Why returns every event with the same correlation_id as the named event,
// in seq order (the M0.5 form of `agt why`). Delegates to the runexec
// sub-package's Runner, which routes to the journal's provenance scan
// (Phase 3.1 C, Day 23).
func (k *Kernel) Why(eventID string) ([]*event.Event, error) { return k.runexec.Why(eventID) }

// Causes returns the causation ancestry of an event, root-first — the
// provenance walk SPEC-01 §7.1 describes. Delegates to the runexec
// sub-package's Runner (Day 23).
func (k *Kernel) Causes(eventID string) ([]*event.Event, error) { return k.runexec.Causes(eventID) }

// ParentOf returns the lead run's correlation for a sub-agent run, or "" if
// childCorr was not spawned via delegation (M42). Delegates to the runexec
// sub-package's Runner (Day 23).
func (k *Kernel) ParentOf(childCorr string) string { return k.runexec.ParentOf(childCorr) }

// Verify replays every event and confirms the BLAKE3 chain is intact.
// Returns nil on success. Delegates to the runexec sub-package's
// Runner (Day 23).
func (k *Kernel) Verify() error { return k.runexec.Verify() }
