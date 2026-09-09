// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

// TestRetryReason_Classifies covers the day-11 helper that
// Runner.RunWithRetry (and the subagent.go retry loop) use to
// build the agent-retry event payload. The classification must
// stay stable because downstream dashboards key off the strings.
func TestRetryReason_Classifies(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "error"},
		{"halted", ErrHalted, "halted"},
		{"canceled", context.Canceled, "canceled"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"unknown", errors.New("boom"), "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryReason(tc.err); got != tc.want {
				t.Errorf("retryReason(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestAgentRetryable_DefaultPolicy covers the default policy
// (empty RetryOn). The contract: halted and canceled NEVER
// auto-retry (operator / Halt initiated); timeout and unknown
// errors do (transient). RunWithRetry and the subagent loop
// rely on this — flipping the default would silently retry
// runs the operator explicitly stopped.
func TestAgentRetryable_DefaultPolicy(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{"halted", false},
		{"canceled", false},
		{"timeout", true},
		{"error", true},
		{"unknown", false},
	}
	for _, tc := range cases {
		if got := agentRetryable(tc.reason, nil); got != tc.want {
			t.Errorf("agentRetryable(%q, nil) = %v, want %v", tc.reason, got, tc.want)
		}
	}
}

// TestAgentRetryable_ExplicitList covers the configured policy
// path. Whitespace-padded entries are trimmed so a config
// authoring slip (" timeout " vs "timeout") does not silently
// disable a retry.
func TestAgentRetryable_ExplicitList(t *testing.T) {
	on := []string{"halted", " timeout "}
	if !agentRetryable("halted", on) {
		t.Errorf("halted in list must be retryable")
	}
	if !agentRetryable("timeout", on) {
		t.Errorf("trimmed ' timeout ' must match 'timeout'")
	}
	if agentRetryable("error", on) {
		t.Errorf("error not in list must NOT be retryable")
	}
}

// TestRetryDelay_NoBackoff covers BaseDelaySec=0 — the contract
// is "try immediately, no timer". RunWithRetry must skip the
// timer wait entirely; otherwise a 0-delay config would still
// hit time.NewTimer + select and cost a goroutine per attempt.
func TestRetryDelay_NoBackoff(t *testing.T) {
	if d := retryDelay(roster.RetryPolicy{}, 5); d != 0 {
		t.Errorf("retryDelay(BaseDelaySec=0) = %v, want 0", d)
	}
}

// TestRetryDelay_LinearBackoff covers the default (no "exponential"
// string) — base delay is constant across attempts. Day-23
// regression: a previous version doubled unconditionally.
func TestRetryDelay_LinearBackoff(t *testing.T) {
	pol := roster.RetryPolicy{BaseDelaySec: 2}
	for _, attempt := range []int{1, 2, 3, 5} {
		if d := retryDelay(pol, attempt); d != 2*time.Second {
			t.Errorf("retryDelay linear attempt=%d = %v, want 2s", attempt, d)
		}
	}
}

// TestRetryDelay_ExponentialBackoff covers Backoff="exponential":
// delay doubles each attempt (1→base, 2→2x, 3→4x, 4→8x).
func TestRetryDelay_ExponentialBackoff(t *testing.T) {
	pol := roster.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential"}
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
	}
	for _, tc := range cases {
		if d := retryDelay(pol, tc.attempt); d != tc.want {
			t.Errorf("retryDelay exp attempt=%d = %v, want %v", tc.attempt, d, tc.want)
		}
	}
}

// TestRetryDelay_MaxCap covers MaxDelaySec — must clamp
// exponential growth so a runaway backoff cannot park a retry
// loop for hours.
func TestRetryDelay_MaxCap(t *testing.T) {
	pol := roster.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential", MaxDelaySec: 3}
	if d := retryDelay(pol, 5); d != 3*time.Second {
		t.Errorf("retryDelay clamped attempt=5 = %v, want 3s", d)
	}
	if d := retryDelay(pol, 2); d != 2*time.Second {
		t.Errorf("retryDelay under cap attempt=2 = %v, want 2s", d)
	}
}
