// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// TestRecoverConn_PanicBecomesErrorNotCrash locks in the per-connection panic
// guard: a panic in a command handler must be turned into an error response to
// the caller, NOT propagate out of the goroutine (which would crash the whole
// daemon — every in-flight run and channel with it). Drives recoverConn the same
// way handleConn defers it.
func TestRecoverConn_PanicBecomesErrorNotCrash(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	s := &Server{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer s.recoverConn(server, "req-9")
		panic("simulated handler panic")
	}()

	// Reading unblocks the (synchronous net.Pipe) write inside recoverConn.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(client).ReadBytes('\n')
	if err != nil {
		t.Fatalf("reading the recovered error response: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Type != RespError {
		t.Errorf("response type = %q, want %q", resp.Type, RespError)
	}
	if resp.ID != "req-9" {
		t.Errorf("response ID = %q, want %q (carried from the request)", resp.ID, "req-9")
	}

	select {
	case <-done:
		// The goroutine returned normally — the panic was recovered, not propagated.
	case <-time.After(2 * time.Second):
		t.Fatal("recover goroutine did not complete (panic escaped?)")
	}
}
