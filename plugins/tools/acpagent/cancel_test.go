// SPDX-License-Identifier: MIT

package acpagent

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

// readerFunc adapts a function to io.Reader.
type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// wedgedTransport simulates a hung external agent: its stdout read blocks
// until close() is called, at which point the read returns EOF — exactly what
// killing a wedged child does to its stdout pipe. close() is idempotent.
func wedgedTransport() *transport {
	unblock := make(chan struct{})
	var once sync.Once
	return &transport{
		out: readerFunc(func([]byte) (int, error) {
			<-unblock
			return 0, io.EOF
		}),
		in:    io.Discard,
		close: func() error { once.Do(func() { close(unblock) }); return nil },
	}
}

func TestACPAgent_ContextTimeoutUnblocksWedgedAgent(t *testing.T) {
	// The external agent never answers initialize; its read blocks forever.
	// Without a ctx-cancel watcher the call would hang past its timeout
	// (exec.Command isn't context-bound and the deferred close only runs
	// after Prompt returns). With it, the timeout tears the read down.
	tool := &Tool{
		Cmd:     "x",
		Cwd:     "/w",
		Timeout: 100 * time.Millisecond,
		dial: func(context.Context, string, string) (*transport, error) {
			return wedgedTransport(), nil
		},
	}
	done := make(chan struct{})
	go func() {
		in, _ := json.Marshal(map[string]string{"task": "hang"})
		_, _ = tool.Invoke(context.Background(), in)
		close(done)
	}()
	select {
	case <-done:
		// Invoke returned promptly — the timeout watcher unblocked the read.
	case <-time.After(5 * time.Second):
		t.Fatal("Invoke wedged well past its 100ms timeout — ctx cancel did not unblock the ACP read")
	}
}

func TestACPAgent_CloseIsIdempotent(t *testing.T) {
	// The Invoke deferred close and the ctx watcher both call close(); a
	// second call must not panic or block.
	tr := wedgedTransport()
	if err := tr.close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := tr.close(); err != nil {
		t.Fatalf("second close should be a no-op, got %v", err)
	}
}
