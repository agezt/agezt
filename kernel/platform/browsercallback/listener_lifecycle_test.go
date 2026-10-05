// SPDX-License-Identifier: MIT

package browsercallback

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestListenerCloseBeforeServeReleasesOwnedAddress(t *testing.T) {
	listener, err := Prepare("127.0.0.1:0", func(context.Context, string, string, string) (bool, string, bool) { return false, "", false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.ln.Close()
	addr := listener.ln.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	rebound, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("EXPECTED: Close releases prepared socket before Serve; ACTUAL: rebinding %s failed: %v", addr, err)
	}
	defer rebound.Close()
	if err := listener.Serve(); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve after Close = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("repeated Close = %v", err)
	}
}

func TestListenerConcurrentCloseAndServeReleaseOwnedAddress(t *testing.T) {
	listener, err := Prepare("127.0.0.1:0", func(context.Context, string, string, string) (bool, string, bool) { return true, "", false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.ln.Close()
	addr := listener.ln.Addr().String()
	start := make(chan struct{})
	served := make(chan error, 1)
	closed := make(chan error, 1)
	go func() { <-start; served <- listener.Serve() }()
	go func() { <-start; closed <- listener.Close() }()
	close(start)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish")
	}
	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Serve/Close cause = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not finish after concurrent Close")
	}
	rebound, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("concurrent Close leaked address %s: %v", addr, err)
	}
	_ = rebound.Close()
}

type closeFailureListener struct {
	net.Listener
	err error
}

func (l closeFailureListener) Close() error { return l.err }

func TestListenerClosePreservesOwnedCleanupFailure(t *testing.T) {
	cause := errors.New("fixture listener close failed")
	for _, tc := range []struct {
		cause error
		want  error
	}{{cause, cause}, {net.ErrClosed, nil}} {
		listener := &Listener{ln: closeFailureListener{err: tc.cause}, srv: &http.Server{}}
		err := listener.Close()
		if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
			t.Fatalf("cleanup cause = %v, want %v", err, tc.want)
		}
	}
}
