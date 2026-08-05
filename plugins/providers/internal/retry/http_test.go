// SPDX-License-Identifier: MIT

package retry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// shortCfg makes retries effectively instant for tests.
func withShortBackoff(t *testing.T) {
	t.Helper()
	old := DefaultConfig
	DefaultConfig.BaseDelay = 0
	DefaultConfig.MaxDelay = 0
	t.Cleanup(func() { DefaultConfig = old })
}

// TestDoHTTP_RetriesTransientThenSucceeds — a 429 first attempt is retried and
// the second attempt's 200 body is returned (the LD-4 contract: every adapter
// funneling through DoHTTP gets the same recovery an OpenAI 429 always had).
func TestDoHTTP_RetriesTransientThenSucceeds(t *testing.T) {
	withShortBackoff(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	body, hdr, err := DoHTTP(context.Background(), srv.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, srv.URL, nil)
	}, 1<<20)
	if err != nil {
		t.Fatalf("DoHTTP: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if hdr == nil {
		t.Error("expected response headers")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls = %d, want 2 (one retry)", got)
	}
}

// TestDoHTTP_TerminalStatusNoRetry — a 400 is not transient: exactly one
// attempt, surfaced as *HTTPError with the body preserved.
func TestDoHTTP_TerminalStatusNoRetry(t *testing.T) {
	withShortBackoff(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "bad input")
	}))
	defer srv.Close()

	_, _, err := DoHTTP(context.Background(), srv.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, srv.URL, nil)
	}, 1<<20)
	var h *HTTPError
	if !errors.As(err, &h) || h.StatusCode != http.StatusBadRequest || h.Body != "bad input" {
		t.Fatalf("err = %v, want HTTPError{400, bad input}", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream calls = %d, want 1 (no retry on 4xx)", got)
	}
}

// TestDoHTTP_BuildErrorTerminal — a request-construction error aborts without
// any upstream call (deterministic failures must not burn retry budget).
func TestDoHTTP_BuildErrorTerminal(t *testing.T) {
	withShortBackoff(t)
	boom := errors.New("boom")
	attempts := 0
	_, _, err := DoHTTP(context.Background(), http.DefaultClient, func() (*http.Request, error) {
		attempts++
		return nil, boom
	}, 1<<20)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if attempts != 1 {
		t.Errorf("build attempts = %d, want 1", attempts)
	}
}

// TestDoHTTPStream_RetriesSetupThenStreams — a 503 setup attempt is retried;
// the caller receives the second attempt's OPEN body and reads the stream.
func TestDoHTTPStream_RetriesSetupThenStreams(t *testing.T) {
	withShortBackoff(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "data: hello\n\n")
	}))
	defer srv.Close()

	resp, err := DoHTTPStream(context.Background(), srv.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, srv.URL, nil)
	}, 1<<20)
	if err != nil {
		t.Fatalf("DoHTTPStream: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if string(raw) != "data: hello\n\n" {
		t.Errorf("stream = %q", raw)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls = %d, want 2 (one setup retry)", got)
	}
}
