// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestClientCallCancellationClosesBlockedConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		close(accepted)
		var one [1]byte
		_, _ = conn.Read(one[:])
		close(closed)
	}()

	client := &Client{addr: ln.Addr().String(), token: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = client.Call(ctx, CmdStatus, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Call ignored cancellation for %s", elapsed)
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("server never received the request")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("cancelled Call did not close the connection")
	}
}
