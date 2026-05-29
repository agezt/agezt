// SPDX-License-Identifier: MIT

package plugin_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/plugin"
)

// TestInvokeWithProgress_StreamsThenTerminal exercises the M1.ss
// progress path end-to-end: spawn a real plugin, call slowwork
// with a progress callback, and verify the callback fires for
// each progress notification before the terminal response arrives.
func TestInvokeWithProgress_StreamsThenTerminal(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	var (
		mu        sync.Mutex
		collected []string
	)
	onProgress := func(s string) {
		mu.Lock()
		collected = append(collected, s)
		mu.Unlock()
	}

	res, err := p.InvokeWithProgress(
		context.Background(),
		"slowwork",
		json.RawMessage(`{}`),
		onProgress,
	)
	if err != nil {
		t.Fatalf("InvokeWithProgress: %v", err)
	}
	if res.Output != "done" {
		t.Errorf("terminal Output = %q, want done", res.Output)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(collected) != 3 {
		t.Fatalf("progress callbacks fired %d times, want 3 (lines=%v)", len(collected), collected)
	}
	// Order IS part of the API contract: callbacks run synchronously
	// on the read-loop goroutine so they arrive in plugin-emit order.
	for i, want := range []string{"step 1 of 3", "step 2 of 3", "step 3 of 3"} {
		if collected[i] != want {
			t.Errorf("progress[%d] = %q, want %q", i, collected[i], want)
		}
	}
}

// TestInvokeWithProgress_NilCallbackDropsProgress verifies that
// passing nil for onProgress doesn't crash and the terminal
// response still arrives correctly — progress lines are silently
// discarded. This is the same behaviour Invoke (without progress)
// gets through the wrapper that passes nil.
func TestInvokeWithProgress_NilCallbackDropsProgress(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	res, err := p.Invoke(context.Background(), "slowwork", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Output != "done" {
		t.Errorf("terminal Output = %q, want done", res.Output)
	}
}

// TestInvokeWithProgress_OtherCallsUnaffected makes sure progress
// routing is per-id: a concurrent Invoke against a different
// request id receives its own terminal response and the slowwork
// progress lines don't bleed into it.
func TestInvokeWithProgress_OtherCallsUnaffected(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// slowwork with progress.
	go func() {
		defer wg.Done()
		_, err := p.InvokeWithProgress(
			context.Background(),
			"slowwork",
			json.RawMessage(`{}`),
			func(string) {},
		)
		if err != nil {
			t.Errorf("slowwork: %v", err)
		}
	}()

	// concurrent echo with no progress consumer.
	go func() {
		defer wg.Done()
		res, err := p.Invoke(context.Background(), "echo", json.RawMessage(`{"text":"hi"}`))
		if err != nil {
			t.Errorf("echo: %v", err)
			return
		}
		if !strings.Contains(res.Output, "hi") {
			t.Errorf("echo Output = %q", res.Output)
		}
	}()

	wg.Wait()
}
