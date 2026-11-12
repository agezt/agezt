// SPDX-License-Identifier: MIT

package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

var errHalted = errors.New("runtime: kernel is halted")

func TestReason_ClassifiesTheFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"halted", errHalted, "halted"},
		{"canceled", context.Canceled, "canceled"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"error", errors.New("boom"), "error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Reason(c.err, errHalted); got != c.want {
				t.Errorf("Reason = %q, want %q", got, c.want)
			}
		})
	}
}

func TestReason_WithoutAHaltSentinel(t *testing.T) {
	// A nil sentinel must not swallow the other classifications.
	if got := Reason(errors.New("boom"), nil); got != "error" {
		t.Errorf("Reason = %q, want error", got)
	}
	if got := Reason(context.Canceled, nil); got != "canceled" {
		t.Errorf("Reason = %q, want canceled", got)
	}
}

// TestReason_ANilErrorIsAnError pins the shape the agent-retry event payload
// depends on: a run that failed with no error at all is still an "error".
func TestReason_ANilErrorIsAnError(t *testing.T) {
	if got := Reason(nil, errHalted); got != "error" {
		t.Errorf("Reason(nil) = %q, want error", got)
	}
}

func TestRetryable_WithoutAPolicy(t *testing.T) {
	// The default policy retries errors and timeouts — never a halted or
	// cancelled run.
	for _, r := range []string{"error", "timeout"} {
		if !Retryable(r, nil) {
			t.Errorf("%q must be retryable by default", r)
		}
	}
	for _, r := range []string{"halted", "canceled"} {
		if Retryable(r, nil) {
			t.Errorf("%q must not be retryable by default", r)
		}
	}
}

func TestRetryable_WithAnAllowlist(t *testing.T) {
	if !Retryable("canceled", []string{"canceled", "error"}) {
		t.Error("an allowlisted reason must be retryable")
	}
	if Retryable("halted", []string{"canceled"}) {
		t.Error("a reason outside the allowlist must not be retryable")
	}
}

func TestDelay_NoBaseDelayIsNoWait(t *testing.T) {
	if got := Delay(agentapi.RetryPolicy{}, 3); got != 0 {
		t.Errorf("Delay = %v, want 0", got)
	}
}

func TestDelay_AFlatBackoff(t *testing.T) {
	got := Delay(agentapi.RetryPolicy{BaseDelaySec: 2}, 4)
	if got != 2*time.Second {
		t.Errorf("Delay = %v, want 2s (a flat backoff does not grow)", got)
	}
}

func TestDelay_AnExponentialBackoffGrows(t *testing.T) {
	pol := agentapi.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential"}
	if got := Delay(pol, 1); got != time.Second {
		t.Errorf("Delay(1) = %v, want 1s", got)
	}
	if got := Delay(pol, 3); got != 4*time.Second {
		t.Errorf("Delay(3) = %v, want 4s", got)
	}
}

func TestDelay_IsClampedToTheMaximum(t *testing.T) {
	pol := agentapi.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential", MaxDelaySec: 3}
	if got := Delay(pol, 10); got != 3*time.Second {
		t.Errorf("Delay = %v, want the 3s cap", got)
	}
}

// TestDelay_AFlatBackoffIsClampedToTheMaximum covers the OTHER clamp: the
// exponential path already caps itself inside the growth loop, so only a flat
// backoff proves the final clamp is applied.
func TestDelay_AFlatBackoffIsClampedToTheMaximum(t *testing.T) {
	pol := agentapi.RetryPolicy{BaseDelaySec: 10, MaxDelaySec: 3}
	if got := Delay(pol, 1); got != 3*time.Second {
		t.Errorf("Delay = %v, want the 3s cap", got)
	}
}

func TestDelay_TheCapIsAppliedInsideTheGrowthLoop(t *testing.T) {
	// A huge attempt count must not overflow the duration.
	pol := agentapi.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential", MaxDelaySec: 60}
	got := Delay(pol, 500)
	if got != 60*time.Second {
		t.Errorf("Delay = %v, want the 60s cap", got)
	}
	if got < 0 {
		t.Errorf("Delay = %v, want a positive duration (the cap is applied inside the loop)", got)
	}
}

// TestDelay_NoOverflow pins the F1 regression: exponential growth must never
// wrap time.Duration's int64 backing to a negative value, even when the
// attempt count is far beyond the validated MaxAttempts=10. Without the
// in-loop cap, base=1s overflows at attempt=35 and base=60s at attempt=29;
// the post-loop MaxDelaySec clamp `if delay > max` does not catch a wrapped
// negative duration, and the caller (time.NewTimer) fires immediately,
// silently destroying the backoff.
//
// The daemon kept these cases for its forwarding wrappers until W3.76ai; the
// rule they pin is the MODULE's, so the rule's tests live here.
func TestDelay_NoOverflow(t *testing.T) {
	t.Run("with cap, very large attempt", func(t *testing.T) {
		pol := agentapi.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential", MaxDelaySec: 60}
		cap := time.Duration(pol.MaxDelaySec) * time.Second
		for _, attempt := range []int{33, 34, 35, 40, 64, 100, 1000} {
			d := Delay(pol, attempt)
			if d < 0 {
				t.Errorf("Delay overflow: attempt=%d returned %v (negative)", attempt, d)
			}
			if d != cap {
				t.Errorf("Delay overflow: attempt=%d returned %v, want cap=%v", attempt, d, cap)
			}
		}
	})
	t.Run("with cap, larger base", func(t *testing.T) {
		pol := agentapi.RetryPolicy{BaseDelaySec: 60, Backoff: "exponential", MaxDelaySec: 86400}
		cap := time.Duration(pol.MaxDelaySec) * time.Second
		for _, attempt := range []int{28, 29, 30, 35, 100, 1000} {
			d := Delay(pol, attempt)
			if d < 0 {
				t.Errorf("Delay overflow: attempt=%d returned %v (negative)", attempt, d)
			}
			if d != cap {
				t.Errorf("Delay overflow: attempt=%d returned %v, want cap=%v", attempt, d, cap)
			}
		}
	})
	t.Run("without cap, very large attempt", func(t *testing.T) {
		// No MaxDelaySec configured: must saturate, never wrap to
		// negative or zero. The pre-fix code returned 0 at attempt=64
		// (int64 wrap of 2^63) and negative values for 35..63.
		pol := agentapi.RetryPolicy{BaseDelaySec: 1, Backoff: "exponential"}
		for _, attempt := range []int{50, 63, 64, 100, 1000} {
			d := Delay(pol, attempt)
			if d <= 0 {
				t.Errorf("Delay overflow: attempt=%d returned %v, want positive duration", attempt, d)
			}
		}
	})
}
