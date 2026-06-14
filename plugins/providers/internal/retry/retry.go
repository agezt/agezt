// SPDX-License-Identifier: MIT

// Package retry provides exponential-backoff retry logic for provider HTTP calls.
package retry

import (
	"context"
	"errors"
	"io"
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
}

func (e *HTTPError) Error() string {
	return "HTTP " + itoa(e.StatusCode) + ": " + e.Body
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

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// Check context before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jitterDelay(delay, cfg.Jitter)):
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

// ReadBody is a helper that reads and closes a response body,
// returning an error for non-2xx status codes.
func ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return io.ReadAll(resp.Body)
}
