package runtime

// Provenance: SPDX-License-Identifier: MIT Runtime runexec retry helpers:
//             retryReason / agentRetryable / retryDelay. Split from
//             runexec_helpers.go during Day 211 god-file refactor (#34). Public API
//             unchanged.
//
// The elision half of this file — the elidedSummary* constants and
// makeElidedSummarizer — moved to the runs module's loop configuration in
// W3.76g: the summarizer is the loop's compaction rule, not the daemon's.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/runs/retry"
)

// retryReason classifies a run failure for the retry policy.
//
// The vocabulary moved to the RUNS MODULE's retry package in W3.76f: the
// delegation engine needed the same three helpers and could not reach the
// daemon's unexported copies. The daemon keeps these thin wrappers so its own
// callers — and the runexec sub-package's API — are unchanged.
func retryReason(err error) string {
	return retry.Reason(err, ErrHalted)
}

// agentRetryable reports whether the given reason matches the policy's
// RetryOn allowlist.
func agentRetryable(reason string, retryOn []string) bool {
	return retry.Retryable(reason, retryOn)
}

// retryDelay computes the wait between attempts.
func retryDelay(pol agentapi.RetryPolicy, attempt int) time.Duration {
	return retry.Delay(pol, attempt)
}
