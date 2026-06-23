// SPDX-License-Identifier: MIT

package retry

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter_Seconds(t *testing.T) {
	if got := ParseRetryAfter("2", time.Now()); got != 2*time.Second {
		t.Fatalf("got %v", got)
	}
	if got := ParseRetryAfter("  5 ", time.Now()); got != 5*time.Second {
		t.Fatalf("trimmed seconds: got %v", got)
	}
	if got := ParseRetryAfter("", time.Now()); got != 0 {
		t.Fatalf("empty should be 0, got %v", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Second).UTC().Format(http.TimeFormat)
	if got := ParseRetryAfter(future, now); got < 29*time.Second || got > 31*time.Second {
		t.Fatalf("date form: got %v", got)
	}
	past := now.Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := ParseRetryAfter(past, now); got != 0 {
		t.Fatalf("past date should be 0, got %v", got)
	}
}

// TestDo_HonorsRetryAfterFloor verifies the directed wait is applied as a floor.
func TestDo_HonorsRetryAfterFloor(t *testing.T) {
	calls := 0
	start := time.Now()
	err := Do(context.Background(), Config{MaxRetries: 1, BaseDelay: time.Millisecond, Jitter: 0},
		func() error {
			calls++
			if calls == 1 {
				return &HTTPError{StatusCode: 429, Body: "slow down", RetryAfter: 120 * time.Millisecond}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	if elapsed := time.Since(start); elapsed < 110*time.Millisecond {
		t.Fatalf("Retry-After floor not honoured; waited only %v", elapsed)
	}
}
