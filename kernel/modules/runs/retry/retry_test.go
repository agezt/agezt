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
