// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestShutdown_ACKsAndClosesChannel verifies the two contracts the
// daemon main loop relies on:
//  1. CmdShutdown returns a clean OK to the client (so `agt
//     shutdown` exits 0 instead of "broken pipe").
//  2. Server.Shutdown() channel closes shortly afterwards, which is
//     how main() learns it should exit.
func TestShutdown_ACKsAndClosesChannel(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdShutdown, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if ok, _ := res["ok"].(bool); !ok {
		t.Errorf("result.ok = %v want true", res["ok"])
	}

	// Channel close happens on a 50ms timer in the handler. Give it
	// up to 500ms — generous margin so a stressed CI box doesn't
	// flake the test, but still far short of any "the channel never
	// closes" hang.
	select {
	case <-srv.Shutdown():
		// expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Shutdown() channel did not close within 500ms of CmdShutdown")
	}
}

// TestShutdown_IdempotentChannelClose covers the corner where a
// second CmdShutdown arrives before the daemon has finished its
// exit sequence. sync.Once must prevent a "close of closed channel"
// panic; the second call should still ACK normally.
func TestShutdown_IdempotentChannelClose(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	for i := 0; i < 3; i++ {
		res, err := c.Call(context.Background(), controlplane.CmdShutdown, nil)
		if err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
		if ok, _ := res["ok"].(bool); !ok {
			t.Errorf("call %d: result.ok = %v want true", i, res["ok"])
		}
	}
	// Channel should be closed (and stay closed) — read from it
	// must return immediately.
	select {
	case <-srv.Shutdown():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Shutdown() channel did not close after 3 shutdown calls")
	}
	// Second read on a closed channel still returns immediately.
	select {
	case <-srv.Shutdown():
	default:
		t.Fatal("closed Shutdown() channel did not yield on second read")
	}
}

// TestShutdown_UnopenedChannelIsAlive sanity-checks that a fresh
// server's Shutdown() channel is open (not nil, not closed). Pure
// constructor test — covers the regression risk of forgetting to
// make() the channel in NewServer.
func TestShutdown_UnopenedChannelIsAlive(t *testing.T) {
	_, srv, _, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ch := srv.Shutdown()
	if ch == nil {
		t.Fatal("Shutdown() returned nil")
	}
	select {
	case <-ch:
		t.Fatal("Shutdown() channel was already closed on fresh server")
	default:
		// expected — channel is open and not ready
	}
}
