// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"testing"
	"time"
)

// TestOnResponse_DuplicateIDDoesNotBlock: a server that emits two responses carrying
// the same id must not wedge the transport read goroutine. onResponse runs ON that
// goroutine, so a blocking send into the cap-1 pending channel would stall every
// future frame (notifications, the death signal) permanently (M428).
func TestOnResponse_DuplicateIDDoesNotBlock(t *testing.T) {
	m := &mcpClient{pending: map[int64]chan *jsonrpcResp{}, done: make(chan struct{})}
	id := int64(1)
	m.pending[id] = make(chan *jsonrpcResp, 1)
	rid := id
	resp := &jsonrpcResp{ID: &rid}

	done := make(chan struct{})
	go func() {
		m.onResponse(resp) // fills the cap-1 buffer
		m.onResponse(resp) // duplicate — must be dropped, not block
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onResponse blocked on a duplicate response id — read loop wedged")
	}
}

// TestMarkDead_KeepsChannelsOpenAndSignalsDone: markDead must NOT close the per-call
// channels (the read goroutine's concurrent send would then panic and crash the
// bridge); it signals death via the shared done channel instead (M428).
func TestMarkDead_KeepsChannelsOpenAndSignalsDone(t *testing.T) {
	m := &mcpClient{pending: map[int64]chan *jsonrpcResp{}, done: make(chan struct{})}
	id := int64(1)
	ch := make(chan *jsonrpcResp, 1)
	m.pending[id] = ch

	m.markDead(errors.New("boom"))

	// Waiting callers wake via done.
	select {
	case <-m.done:
	default:
		t.Fatal("markDead must close done so waiting callers wake")
	}
	// The per-call channel must remain open: a late send from the read goroutine must
	// not panic. (The old code closed it → this send panics, failing the test.)
	rid := id
	ch <- &jsonrpcResp{ID: &rid}
}
