// SPDX-License-Identifier: MIT

// Package retry provides exponential-backoff retry logic for provider HTTP calls.
package retry

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"time"
)

// Config controls retry behavior.
type Config struct {
	// MaxRetries is the maximum number of retry attempts (default 3).
	MaxRetries int
	// BaseDelay is the initial delay before the first retry (default 500ms).
	BaseDelay time.Duration
	// MaxDelay is the maximum delay between retries (default 30s).
	MaxDelay time.Duration
	// Multiplier is the exponential multiplier (default 2.0).
	Multiplier float64
	// Jitter fraction (0-1) added to delay to prevent thundering herd (default 0.1).
	Jitter float64
}

var DefaultConfig = Config{
	MaxRetries: 3,
	BaseDelay:  500 * time.Millisecond,
	MaxDelay:   30 * time.Second,
	Multiplier: 2.0,
	Jitter:     0.1,
}

// TransientError wraps an error that may be transient and worth retrying.
type TransientError struct {
	Err error
}

func (e *TransientError) Error() string { return e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

// IsTransient reports whether err is a transient error worth retrying.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation is not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Network errors — net.Error is an interface with Timeout() method
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// HTTP errors
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Transient()
	}
	return false
}

// HTTPError represents an HTTP error with transient classification.
type HTTPError struct {
	StatusCode int
	Body       string
	// RetryAfter, when > 0, is the server-directed minimum wait before retrying
	// (parsed from the Retry-After header, M997). Do honours it as a delay floor.
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return "HTTP " + itoa(e.StatusCode) + ": " + e.Body
}

// NewHTTPError builds an HTTPError from a response, parsing the Retry-After
// header so 429/503 backoff can honour the server's directive. body is the
// already-read response body.
func NewHTTPError(resp *http.Response, body string) *HTTPError {
	e := &HTTPError{StatusCode: resp.StatusCode, Body: body}
	if ra := ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ra > 0 {
		e.RetryAfter = ra
	}
	return e
}

// ParseRetryAfter parses an HTTP Retry-After header value (RFC 7231): either a
// non-negative number of seconds, or an HTTP-date. now anchors the date form.
// Returns 0 for empty/invalid values or dates in the past.
func ParseRetryAfter(v string, now time.Time) time.Duration {
	v = trimSpace(v)
	if v == "" {
		return 0
	}
	// delta-seconds form.
	if secs, ok := atoiNonNeg(v); ok {
		return time.Duration(secs) * time.Second
	}
	// HTTP-date form.
	for _, layout := range []string{http.TimeFormat, time.RFC1123, time.RFC1123Z, time.RFC850, time.ANSIC} {
		if t, err := time.Parse(layout, v); err == nil {
			if d := t.Sub(now); d > 0 {
				return d
			}
			return 0
		}
	}
	return 0
}

// RetryAfterOf extracts the RetryAfter directive from an error chain, or 0.
func RetryAfterOf(err error) time.Duration {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.RetryAfter
	}
	return 0
}

// Transient reports whether this HTTP error is transient and worth retrying.
func (e *HTTPError) Transient() bool {
	// 429 Rate Limit — retry after backoff
	if e.StatusCode == http.StatusTooManyRequests {
		return true
	}
	// 5xx Server Errors — retry after backoff
	if e.StatusCode >= 500 && e.StatusCode < 600 {
		return true
	}
	// 2xx — not errors, but we still wrap for consistency
	// 4xx — usually not retryable (except 429)
	return false
}

// Do executes fn with exponential backoff retry. It retries transient errors
// up to MaxRetries times. Returns the last error if all retries fail.
// fn should return a transient error (using NewTransientError) for retryable
// failures so IsTransient can classify them.
func Do(ctx context.Context, cfg Config, fn func() error) error {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultConfig.MaxRetries
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = DefaultConfig.BaseDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = DefaultConfig.MaxDelay
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = DefaultConfig.Multiplier
	}

	var lastErr error
	delay := cfg.BaseDelay
	var retryAfter time.Duration // server-directed floor from the previous error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := jitterDelay(delay, cfg.Jitter)
			// Honour a Retry-After directive as a floor, capped so a hostile or
			// mis-set header can't stall the call indefinitely.
			if retryAfter > wait {
				wait = retryAfter
				if ceil := 2 * time.Minute; wait > ceil {
					wait = ceil
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			// Exponential backoff
			delay = time.Duration(float64(delay) * cfg.Multiplier)
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		if !IsTransient(err) {
			return err
		}

		lastErr = err
		retryAfter = RetryAfterOf(err)
	}

	return lastErr
}

// jitterDelay adds random jitter to delay to prevent thundering herd.
func jitterDelay(delay time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return delay
	}
	// rand.Int63n is safe to call without locking since we're single-threaded per call
	jitterNs := int64(float64(delay) * jitter * (2*rand.Float64() - 1))
	return delay + time.Duration(jitterNs)
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// trimSpace trims leading/trailing ASCII spaces and tabs without importing strings.
func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// atoiNonNeg parses a non-negative base-10 integer; ok=false for empty or any
// non-digit content (so an HTTP-date falls through to date parsing).
func atoiNonNeg(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
