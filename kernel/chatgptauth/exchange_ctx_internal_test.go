// SPDX-License-Identifier: MIT

package chatgptauth

// Regression test for BUG dropped-r.Context at the chatgptauth layer:
// Manager.ExchangeCode must propagate the caller's ctx through to
// the outbound HTTP request. The control plane's providerCallback
// previously dropped the request context by passing
// context.Background(); after the audit fix it passes r.Context(),
// and this lower-layer test guarantees that whichever ctx arrives
// at Manager.ExchangeCode reaches postToken and the HTTP client
// (so a fix at the call site is actually effective end-to-end).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExchangeCode_AbortsOnContextCancel(t *testing.T) {
	// Token endpoint that holds the response and writes one byte at
	// a time until the test closes its end. The test's cancel() must
	// unwind the client-side httpClient.Do even though the server is
	// still streaming a (deliberately slow) response.
	gotReq := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotReq <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// Drip the body. Don't watch r.Context() — the server-side
		// context is cancelled on TCP close, not on the client
		// cancelling its context, so waiting for it would make
		// the test deadlocked on graceful httptest.Server.Close.
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 200; i++ { // ~20s of data
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
			if _, err := w.Write([]byte(".")); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	// Swap token endpoint + http client to reach loopback (the default
	// netguard client blocks loopback, and we want this test hermetic).
	prevEP := tokenEP
	prevClient := httpClientFor
	tokenEP = srv.URL
	httpClientFor = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout}
	}
	t.Cleanup(func() {
		tokenEP = prevEP
		httpClientFor = prevClient
	})

	m := NewManager(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.ExchangeCode(ctx, "code", "verifier")
	}()

	select {
	case <-gotReq:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("token endpoint never received the request")
	}

	cancelStart := time.Now()
	cancel()

	select {
	case err := <-errCh:
		elapsed := time.Since(cancelStart)
		// postToken uses WithTimeout(ctx, 20s); the dropped-ctx bug
		// would leave the call blocked for the full 20s. A correct
		// implementation unwinds within ~50ms of cancel.
		if elapsed > 1*time.Second {
			t.Errorf("ExchangeCode honoured %v after ctx cancel — expected <1s (dropped ctx leaked)", elapsed)
		}
		// We don't pin the exact error string (depends on the
		// net/http transport); just confirm the cancel unwound it.
		_ = err // err may be nil if cancel raced the success path; the
		//   timing assertion above is what proves ctx was respected.
	case <-time.After(3 * time.Second):
		t.Fatal("ExchangeCode did not return within 3s of ctx cancel (dropped ctx leaked)")
	}
}
