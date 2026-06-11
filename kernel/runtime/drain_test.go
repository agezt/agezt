// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// stubbornTool ignores context cancellation and sleeps a fixed duration —
// modelling a tool mid-write that a Halt cannot interrupt.
type stubbornTool struct {
	sleep    time.Duration
	started  chan struct{}
	finished atomic.Bool
}

func (s *stubbornTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "stubborn", Description: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (s *stubbornTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	close(s.started)
	time.Sleep(s.sleep) // deliberately ignores ctx
	s.finished.Store(true)
	return agent.Result{Output: "done"}, nil
}

// TestClose_DrainsInFlightRun (M883): Close waits for an in-flight run to
// settle (default 5s window) before tearing down the stores it writes to —
// a tool that honours cancellation late no longer races store teardown.
func TestClose_DrainsInFlightRun(t *testing.T) {
	tool := &stubbornTool{sleep: 300 * time.Millisecond, started: make(chan struct{})}
	prov := mock.New(
		mock.ToolUse("c1", "stubborn", map[string]any{}),
		mock.FinalText("never reached"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools:    map[string]agent.Tool{"stubborn": tool},
		// The test tool maps to an unknown capability; without UnknownAllow the
		// policy gate would deny it and the tool would never run.
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _ = k.Run(context.Background(), "hold the line")
	}()
	<-tool.started

	if err := k.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !tool.finished.Load() {
		t.Error("Close returned while the in-flight tool was still running — no drain happened")
	}
	wg.Wait()
}

// TestClose_DrainTimeoutBounds (M883): a run wedged in a cancel-ignoring tool
// cannot block shutdown past ShutdownDrainTimeout.
func TestClose_DrainTimeoutBounds(t *testing.T) {
	tool := &stubbornTool{sleep: 2 * time.Second, started: make(chan struct{})}
	prov := mock.New(
		mock.ToolUse("c1", "stubborn", map[string]any{}),
		mock.FinalText("never reached"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:              t.TempDir(),
		Provider:             prov,
		Tools:                map[string]agent.Tool{"stubborn": tool},
		ShutdownDrainTimeout: 100 * time.Millisecond,
		Edict:                edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _ = k.Run(context.Background(), "wedge")
	}()
	<-tool.started

	t0 := time.Now()
	if err := k.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(t0); elapsed > 1500*time.Millisecond {
		t.Errorf("Close took %s, want ~100ms drain bound (not the tool's full 2s)", elapsed)
	}
	// Let the wedged run unwind before the test exits, so a post-teardown
	// panic (if any regression introduces one) is caught here.
	wg.Wait()
}
