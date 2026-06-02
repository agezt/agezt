// SPDX-License-Identifier: MIT

package plugin

// White-box tests for the bounded callback dispatcher (M181): a plugin
// must not be able to spawn unbounded host-callback goroutines by
// flooding host/invoke frames. dispatchCallback acquires a slot from a
// fixed-size semaphore; over the cap it rejects inline.

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuf is a concurrency-safe io.WriteCloser standing in for the
// plugin's stdin, so handleCallback's response write (which may run on a
// spawned goroutine) can be observed without a race.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) Close() error { return nil }
func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Over the cap: a callback dispatched while the semaphore is full is
// rejected inline with ErrTooManyCallbacks and consumes NO slot (no
// goroutine spawned, nothing leaked).
func TestDispatchCallback_RejectsOverCap(t *testing.T) {
	out := &syncBuf{}
	p := &Plugin{cbSem: make(chan struct{}, 2), stdin: out}
	p.cbSem <- struct{}{} // fill both
	p.cbSem <- struct{}{}

	p.dispatchCallback(inboundFrame{ID: "cb-1", Method: MethodHostInvoke})

	var resp Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp); err != nil {
		t.Fatalf("parse rejection %q: %v", out.String(), err)
	}
	if resp.ID != "cb-1" || !strings.Contains(resp.Error, "too many concurrent callbacks") {
		t.Errorf("rejection = %+v; want ErrTooManyCallbacks for cb-1", resp)
	}
	if len(p.cbSem) != 2 {
		t.Errorf("reject consumed/leaked a slot: len=%d want 2", len(p.cbSem))
	}
}

// Under the cap: the callback is accepted (a slot is acquired then
// released by handleCallback), and it is NOT rejected. With HostTools
// nil, handleCallback surfaces the callbacks-disabled error — proving it
// took the accept path, not the over-cap reject path.
func TestDispatchCallback_AcceptsUnderCap(t *testing.T) {
	out := &syncBuf{}
	p := &Plugin{cbSem: make(chan struct{}, 2), stdin: out}

	p.dispatchCallback(inboundFrame{ID: "cb-1", Method: MethodHostInvoke})

	// Wait for handleCallback to finish (slot released).
	deadline := time.Now().Add(2 * time.Second)
	for len(p.cbSem) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("callback slot never released — goroutine stuck or slot leaked")
		}
		time.Sleep(time.Millisecond)
	}

	var resp Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if resp.ID != "cb-1" {
		t.Fatalf("no response for cb-1: %q", out.String())
	}
	if strings.Contains(resp.Error, "too many concurrent callbacks") {
		t.Errorf("under-cap callback wrongly rejected: %+v", resp)
	}
	if !strings.Contains(resp.Error, "not enabled") {
		t.Errorf("expected callbacks-disabled error (accept path ran handleCallback), got %+v", resp)
	}
}
