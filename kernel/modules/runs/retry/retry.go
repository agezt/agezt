// SPDX-License-Identifier: MIT

// Package retry is the RUNS MODULE's retry vocabulary: how a failed run is
// classified, whether a policy allows another attempt, and how long the
// backoff waits.
//
// It is a shared vocabulary, not an engine: the delegation engine (kernel/
// modules/runs/delegation) uses it for a delegated child's retries, and the
// daemon's root-run retry uses it through the same functions. Before W3.76f
// the three helpers lived in the daemon's runtime package, where the
// delegation engine could not reach them — the module would have had to
// either duplicate them or ask the daemon for every decision. One package,
// one implementation, two callers.
//
// The halt sentinel is a PARAMETER, not a constant: the daemon owns the
// identity of its halt error, and the module asks the daemon for it. A halted
// run is therefore classified exactly as the daemon classifies it.
package retry

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// Reason classifies a run failure for the retry policy: "halted",
// "canceled", "timeout" or "error".
//
// The halted sentinel is passed in because the daemon owns it: the daemon's
// runtime package wraps this call with its own ErrHalted, and the delegation
// engine passes the sentinel it got from its host.
func Reason(err error, halted error) string {
	switch {
	case halted != nil && errors.Is(err, halted):
		return "halted"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "error"
	}
}

// Retryable reports whether a reason may be retried under the policy. An
// empty policy retries only "error" and "timeout" — a cancelled or halted run
// is never retried unless the policy says so explicitly.
func Retryable(reason string, retryOn []string) bool {
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

// Delay returns the backoff before the given attempt: the policy's base
// delay, doubled per attempt when the backoff is exponential, clamped to the
// policy's maximum delay.
func Delay(pol agentapi.RetryPolicy, attempt int) time.Duration {
	base := time.Duration(pol.BaseDelaySec) * time.Second
	if base <= 0 {
		return 0
	}
	delay := base
	if strings.TrimSpace(pol.Backoff) == "exponential" {
		// Cap inside the loop so the next *= 2 cannot overflow time.Duration's
		// int64 backing (base=1s overflows at attempt=35, base=60s at attempt=29;
		// the post-loop clamp below does not catch a wrapped negative value).
		var hardCap time.Duration
		if pol.MaxDelaySec > 0 {
			hardCap = time.Duration(pol.MaxDelaySec) * time.Second
		} else {
			hardCap = math.MaxInt64 / 2
		}
		for i := 1; i < attempt; i++ {
			if delay >= hardCap {
				return hardCap
			}
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
