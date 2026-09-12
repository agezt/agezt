// SPDX-License-Identifier: MIT

// Runtime runexec: Run/RunAssured/RunWithRetry + run-state setup/cleanup + FoldRunTools + CompleteAgentLifecycle.
// Code extracted from runexec.go during the Day-122 god-file split.
// Public API unchanged.
package runtime


import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/roster"
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

