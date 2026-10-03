// SPDX-License-Identifier: MIT

package retry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

// Every adapter wraps a failed client.Do in *TransientError — that wrapper is
// the adapters' whole signal that "the request never got an answer, try
// again". IsTransient used to ignore it and only recognise net.Error timeouts,
// so connection refused / reset / DNS failures (none of which are timeouts)
// failed the run on the first attempt despite every adapter marking them
// retryable.
func TestIsTransientHonoursTheTransientWrapper(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"wrapped connection refused", &TransientError{Err: refused}, true},
		{"wrapped DNS failure", &TransientError{Err: &net.DNSError{Err: "no such host", Name: "api.example"}}, true},
		{"wrapper further wrapped", fmt.Errorf("provider: %w", &TransientError{Err: errors.New("connection reset by peer")}), true},
		{"unwrapped connection refused (not marked)", refused, false},
		{"wrapped cancellation stays final", &TransientError{Err: context.Canceled}, false},
		{"wrapped deadline stays final", &TransientError{Err: context.DeadlineExceeded}, false},
		{"plain error", errors.New("boom"), false},
	}
	for _, c := range cases {
		if got := IsTransient(c.err); got != c.want {
			t.Errorf("%s: IsTransient = %v, want %v", c.name, got, c.want)
		}
	}
}

// End-to-end through DoHTTP: the first attempt hits a closed port (connection
// refused — a non-timeout dial error), the next ones reach a live server.
func TestDoHTTPRetriesConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := ln.Addr().String()
	_ = ln.Close() // nothing listens here any more: dialing it is refused

	oldBase := DefaultConfig.BaseDelay
	DefaultConfig.BaseDelay = time.Millisecond
	t.Cleanup(func() { DefaultConfig.BaseDelay = oldBase })

	attempts := 0
	body, _, err := DoHTTP(context.Background(), srv.Client(), func() (*http.Request, error) {
		attempts++
		url := srv.URL
		if attempts == 1 {
			url = "http://" + deadAddr
		}
		return http.NewRequest(http.MethodGet, url, nil)
	}, 1<<10)
	if err != nil {
		t.Fatalf("DoHTTP failed after %d attempt(s): %v — a refused connection must be retried", attempts, err)
	}
	if string(body) != "ok" || attempts != 2 {
		t.Fatalf("body=%q attempts=%d, want ok after 2 attempts", body, attempts)
	}
}
