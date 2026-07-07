// SPDX-License-Identifier: MIT

package retry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// --- TransientError ---

func TestTransientError_ErrorAndUnwrap(t *testing.T) {
	base := errors.New("boom")
	te := &TransientError{Err: base}
	if te.Error() != "boom" {
		t.Fatalf("Error() = %q", te.Error())
	}
	if te.Unwrap() != base {
		t.Fatalf("Unwrap did not return base error")
	}
	if !errors.Is(te, base) {
		t.Fatalf("errors.Is should match wrapped base")
	}
}

// --- IsTransient across all branches ---

type timeoutErr struct{ to bool }

func (e timeoutErr) Error() string   { return "timeout" }
func (e timeoutErr) Timeout() bool   { return e.to }
func (e timeoutErr) Temporary() bool { return e.to }

func TestIsTransient_AllBranches(t *testing.T) {
	if IsTransient(nil) {
		t.Fatal("nil should not be transient")
	}
	if IsTransient(context.Canceled) {
		t.Fatal("context.Canceled must not be transient")
	}
	if IsTransient(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded must not be transient")
	}
	// net.Error with Timeout()==true is transient.
	var ne net.Error = timeoutErr{to: true}
	if !IsTransient(ne) {
		t.Fatal("timeout net.Error should be transient")
	}
	// net.Error with Timeout()==false falls through to false.
	if IsTransient(net.Error(timeoutErr{to: false})) {
		t.Fatal("non-timeout net.Error should not be transient")
	}
	// HTTPError transient (429).
	if !IsTransient(&HTTPError{StatusCode: 429}) {
		t.Fatal("429 HTTPError should be transient")
	}
	// HTTPError non-transient (400).
	if IsTransient(&HTTPError{StatusCode: 400}) {
		t.Fatal("400 HTTPError should not be transient")
	}
	// Plain error → not transient.
	if IsTransient(errors.New("plain")) {
		t.Fatal("plain error should not be transient")
	}
}

// --- HTTPError.Error / itoa ---

func TestHTTPError_Error(t *testing.T) {
	e := &HTTPError{StatusCode: 503, Body: "unavailable"}
	if got, want := e.Error(), "HTTP 503: unavailable"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	// itoa(0) path.
	z := &HTTPError{StatusCode: 0, Body: "x"}
	if got, want := z.Error(), "HTTP 0: x"; got != want {
		t.Fatalf("zero status Error() = %q, want %q", got, want)
	}
}

// --- NewHTTPError ---

func TestNewHTTPError_WithRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": []string{"3"}},
	}
	e := NewHTTPError(resp, "slow down")
	if e.StatusCode != 429 || e.Body != "slow down" {
		t.Fatalf("unexpected fields: %+v", e)
	}
	if e.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %v, want 3s", e.RetryAfter)
	}
}

func TestNewHTTPError_NoRetryAfter(t *testing.T) {
	resp := &http.Response{StatusCode: 500, Header: http.Header{}}
	e := NewHTTPError(resp, "boom")
	if e.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %v, want 0", e.RetryAfter)
	}
}

// --- RetryAfterOf ---

func TestRetryAfterOf(t *testing.T) {
	if got := RetryAfterOf(errors.New("plain")); got != 0 {
		t.Fatalf("plain error RetryAfterOf = %v, want 0", got)
	}
	he := &HTTPError{StatusCode: 429, RetryAfter: 7 * time.Second}
	if got := RetryAfterOf(he); got != 7*time.Second {
		t.Fatalf("RetryAfterOf = %v, want 7s", got)
	}
	// Wrapped.
	wrapped := &TransientError{Err: he}
	if got := RetryAfterOf(wrapped); got != 7*time.Second {
		t.Fatalf("wrapped RetryAfterOf = %v, want 7s", got)
	}
}

// --- Transient ---

func TestHTTPError_Transient(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{http.StatusTooManyRequests, true},
		{500, true},
		{503, true},
		{599, true},
		{400, false},
		{404, false},
		{200, false},
	}
	for _, c := range cases {
		if got := (&HTTPError{StatusCode: c.status}).Transient(); got != c.want {
			t.Fatalf("status %d Transient()=%v, want %v", c.status, got, c.want)
		}
	}
}

// --- Do: success first try ---

func TestDo_SuccessImmediate(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{MaxRetries: 3, BaseDelay: time.Millisecond, Jitter: 0}, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

// --- Do: non-transient error returns immediately ---

func TestDo_NonTransientStops(t *testing.T) {
	calls := 0
	sentinel := errors.New("fatal")
	err := Do(context.Background(), Config{MaxRetries: 5, BaseDelay: time.Millisecond, Jitter: 0}, func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want sentinel", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1 (non-transient must not retry)", calls)
	}
}

// --- Do: exhausts retries and returns last error ---

func TestDo_ExhaustsRetries(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{MaxRetries: 2, BaseDelay: time.Millisecond, Jitter: 0}, func() error {
		calls++
		return &HTTPError{StatusCode: 503, Body: "down"}
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// initial + MaxRetries = 3 calls.
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
}

// --- Do: applies config defaults when zero ---

func TestDo_ZeroConfigDefaults(t *testing.T) {
	calls := 0
	// All zero → defaults filled. Use success on 2nd call to keep it fast-ish,
	// but BaseDelay default (500ms) makes one wait; keep MaxRetries default.
	// To avoid a long wait, succeed on first call.
	err := Do(context.Background(), Config{}, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

// --- Do: MaxDelay cap on exponential backoff ---

func TestDo_MaxDelayCap(t *testing.T) {
	calls := 0
	// BaseDelay above MaxDelay so the very first computed delay is capped.
	cfg := Config{MaxRetries: 3, BaseDelay: 2 * time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 100, Jitter: 0}
	err := Do(context.Background(), cfg, func() error {
		calls++
		if calls < 3 {
			return &HTTPError{StatusCode: 500}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
}

// --- Do: context cancellation during wait ---

func TestDo_ContextCanceledDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	// Long delay so cancel fires before time.After.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	err := Do(ctx, Config{MaxRetries: 3, BaseDelay: time.Hour, Jitter: 0}, func() error {
		calls++
		return &HTTPError{StatusCode: 503}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

// --- Do: Retry-After ceiling cap (>2min clamped) ---

func TestDo_RetryAfterCeilingClamp(t *testing.T) {
	calls := 0
	start := time.Now()
	// RetryAfter huge, but delay is tiny; the ceiling is 2min. We can't wait 2min,
	// so cancel the context shortly after the wait begins to confirm the floor is
	// applied (wait would be 2min → cancel interrupts). This exercises the clamp branch.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(15 * time.Millisecond)
		cancel()
	}()
	err := Do(ctx, Config{MaxRetries: 1, BaseDelay: time.Millisecond, Jitter: 0}, func() error {
		calls++
		return &HTTPError{StatusCode: 429, RetryAfter: time.Hour}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled (proves 2min floor was set)", err)
	}
	if time.Since(start) > time.Minute {
		t.Fatal("waited too long; ceiling clamp likely broken")
	}
}

// --- jitterDelay ---

func TestJitterDelay(t *testing.T) {
	// jitter <= 0 → returns delay unchanged.
	if got := jitterDelay(100*time.Millisecond, 0); got != 100*time.Millisecond {
		t.Fatalf("jitter 0 changed delay: %v", got)
	}
	// jitter > 0 → within +/- jitter fraction.
	base := 100 * time.Millisecond
	for i := 0; i < 200; i++ {
		got := jitterDelay(base, 0.1)
		lo := time.Duration(float64(base) * 0.9)
		hi := time.Duration(float64(base) * 1.1)
		if got < lo || got > hi {
			t.Fatalf("jittered delay %v out of [%v,%v]", got, lo, hi)
		}
	}
}

// --- atoiNonNeg edge cases via ParseRetryAfter ---

func TestParseRetryAfter_InvalidAndNegative(t *testing.T) {
	now := time.Now()
	// Non-numeric, non-date → 0.
	if got := ParseRetryAfter("not-a-number", now); got != 0 {
		t.Fatalf("garbage = %v, want 0", got)
	}
	// Negative number → not parsed as delta-seconds (atoiNonNeg rejects '-'),
	// and not a date → 0.
	if got := ParseRetryAfter("-5", now); got != 0 {
		t.Fatalf("negative = %v, want 0", got)
	}
	// Numeric with trailing junk → rejected by atoiNonNeg → 0.
	if got := ParseRetryAfter("12x", now); got != 0 {
		t.Fatalf("12x = %v, want 0", got)
	}
	// Zero seconds → valid, 0 duration.
	if got := ParseRetryAfter("0", now); got != 0 {
		t.Fatalf("0 = %v, want 0", got)
	}
}

// TestAtoiNonNeg_Direct exercises atoiNonNeg branches not reachable through
// ParseRetryAfter (which pre-trims and rejects empty input before calling it).
func TestAtoiNonNeg_Direct(t *testing.T) {
	// Empty input → ok=false.
	if n, ok := atoiNonNeg(""); ok || n != 0 {
		t.Fatalf("empty: n=%d ok=%v, want 0,false", n, ok)
	}
	// Valid digits → parsed value.
	if n, ok := atoiNonNeg("42"); !ok || n != 42 {
		t.Fatalf("42: n=%d ok=%v, want 42,true", n, ok)
	}
	// Leading zero still valid.
	if n, ok := atoiNonNeg("007"); !ok || n != 7 {
		t.Fatalf("007: n=%d ok=%v, want 7,true", n, ok)
	}
	// Non-digit char → ok=false.
	if _, ok := atoiNonNeg("1a"); ok {
		t.Fatalf("1a should be rejected")
	}
	// Leading non-digit → ok=false.
	if _, ok := atoiNonNeg("a1"); ok {
		t.Fatalf("a1 should be rejected")
	}
}
