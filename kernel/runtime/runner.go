// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime runner: Run/RunAssured/RunWithRetry + run-state setup/cleanup
//             + FoldRunTools + CompleteAgentLifecycle. Code extracted from
//             runexec.go during the Day-122 god-file split. Public API unchanged.

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/runs/api"
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
//   configMu (light config) < runsMu < fanoutMu < treeMu < steersMu < spawnsMu
//
// The deferred cleanup at the bottom of RunWith takes all five
// (runsMu → fanoutMu → treeMu → steersMu → spawnsMu) in this exact
// order and releases them in reverse. Any new locking site added in
// this file MUST respect the same order, or `go test -race` will trip.
// The Live-steering registration right after the runsMu unlock also
// follows the rule (steersMu acquired AFTER runsMu is released — see
// the comment at runtime.go:910-919 for the full history).

// The RUN ENGINE is the runs module's, and the daemon holds it (W3.76ac).
//
// The run engine's own operations live in the runner package
// (kernel/modules/runs/runner); the daemon used to answer for them under
// names of its own: Run, RunWith, RunWithRetry, RunAssured and
// CompleteAgentLifecycle were five verbs over the module's own body — the run
// engine already answered in exactly these names, so the daemon's five were a
// second spelling of one family, and every caller above had to learn the
// daemon's.
//
// The daemon hands over the module's FACE (RunnerService) instead, and the
// callers above ask the module. The daemon keeps what is the HOST's own: the
// correlation id a run carries (NewCorrelation) and the run's own setup and
// cleanup.

// RunnerService returns the RUNS MODULE's run engine — the module's face.
//
// The daemon holds no verb per run operation any more (W3.76ac): the run
// engine's body is the module's, and the daemon hands the module over
// instead of answering for it. The verbs are the MODULE's names, so one
// operation has one name.
func (k *Kernel) RunnerService() api.RunnerService { return k.runner }

// ClaimResumeTicket is the public surface for the resume-ticket
// ownership dance. Returns the (possibly ticket-bearing) ctx and
// whether THIS frame owns the ticket (true = caller must finalize).
// Wraps claimResumeTicket in resume.go so the runner package
// can drive it through KernelAPI (Day 23).
func (k *Kernel) ClaimResumeTicket(ctx context.Context, corr, intent, kind string, assureBudget int) (context.Context, bool) {
	return k.claimResumeTicket(ctx, corr, intent, kind, assureBudget)
}

// FinalizeResumeTicket is the public surface for clearing or
// keeping a resume ticket on run exit. Honours the registry's suspend latch so
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
func (k *Kernel) AgentRetryable(reason string, retryOn []string) bool {
	return agentRetryable(reason, retryOn)
}

// RetryDelay computes the wait between attempts. Exponential
// backoff doubles each attempt; MaxDelaySec caps the result.
// Zero base delay = no wait (try immediately).
func (k *Kernel) RetryDelay(pol agentapi.RetryPolicy, attempt int) time.Duration {
	return retryDelay(pol, attempt)
}
