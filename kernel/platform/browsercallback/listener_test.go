// SPDX-License-Identifier: MIT

package browsercallback

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreparedListenerPreservesBindServerOptionsAndCallerOwnedServe(t *testing.T) {
	var published atomic.Bool
	listener, err := Prepare("127.0.0.1:0", func(_ context.Context, code, state, denial string) (bool, string, bool) {
		if !published.Load() || code != "fixture" || state != "owned" || denial != "" {
			t.Errorf("callback entered without published ownership or lost query: %q %q %q", code, state, denial)
		}
		return true, "", false
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.ln.Close()
	defer listener.Close()
	if listener.srv.ReadHeaderTimeout != 10*time.Second || listener.srv.ReadTimeout != 0 || listener.srv.WriteTimeout != 0 {
		t.Fatalf("server options changed: %+v", listener.srv)
	}
	if _, err := Prepare(listener.ln.Addr().String(), func(context.Context, string, string, string) (bool, string, bool) { return false, "", false }, nil); err == nil {
		t.Fatal("bound address unexpectedly shared")
	}
	published.Store(true)
	done := make(chan error, 1)
	go func() { done <- listener.Serve() }()
	client := &http.Client{Timeout: 2 * time.Second}
	for _, tc := range []struct {
		path string
		code int
	}{{"/auth/callback?code=fixture&state=owned", 200}, {"/not-callback", 404}} {
		response, err := client.Get("http://" + listener.ln.Addr().String() + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != tc.code || (tc.code == 200 && !strings.Contains(string(body), "Signed in ✓")) {
			t.Fatalf("response for %s: %d %q %v", tc.path, response.StatusCode, body, err)
		}
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve close cause changed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after Close")
	}
}

func TestPreparedListenerPreservesNilResultOnBindFailure(t *testing.T) {
	listener, err := Prepare("invalid-address", func(context.Context, string, string, string) (bool, string, bool) { return false, "", false }, nil)
	if listener != nil || err == nil {
		t.Fatalf("bind failure changed: %v %v", listener, err)
	}
}
