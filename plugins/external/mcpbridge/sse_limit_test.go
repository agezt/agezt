// SPDX-License-Identifier: MIT

package main

// Live proof for M185 on the SSE transport: an MCP server that sends an
// over-cap event frame must tear the transport down (onTransportDead
// with errMCPFrameTooLarge), not OOM the bridge. Uses the mockMCPServer
// / captureDeliver harness from sse_transport_test.go and temporarily
// lowers maxMCPFrameBytes so the bound trips on a small payload.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSSETransport_RejectsOversizedFrame(t *testing.T) {
	old := maxMCPFrameBytes
	maxMCPFrameBytes = 1024
	defer func() { maxMCPFrameBytes = old }()

	srv := newMockMCPServer(t)
	dc := newCaptureDeliver()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// The endpoint event (data: /messages) is well under the lowered cap,
	// so construction still succeeds.
	tx, err := newSSETransport(ctx, srv.srv.URL+"/sse", dc)
	if err != nil {
		t.Fatalf("newSSETransport: %v", err)
	}
	defer tx.close()

	// Now stream an event whose data line blows past the 1 KiB cap.
	srv.events <- "event: message\ndata: " + strings.Repeat("Z", 4096) + "\n\n"

	select {
	case <-dc.deadCh:
		dc.mu.Lock()
		derr := dc.dead
		dc.mu.Unlock()
		if !errors.Is(derr, errMCPFrameTooLarge) {
			t.Errorf("transport dead err = %v; want errMCPFrameTooLarge", derr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transport did not die on an oversized SSE frame")
	}
}
