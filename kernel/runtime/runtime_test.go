// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func newKernel(t *testing.T, prov agent.Provider) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools:    map[string]agent.Tool{"shell": shell.New()},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func TestRun_Simple(t *testing.T) {
	k := newKernel(t, mock.New(mock.FinalText("done")))
	ans, corr, err := k.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "done" {
		t.Errorf("ans=%q want done", ans)
	}
	if !strings.HasPrefix(corr, "run-") {
		t.Errorf("corr=%q should start with run-", corr)
	}
}

func TestHalt_PreventsNewRuns(t *testing.T) {
	k := newKernel(t, mock.New(mock.FinalText("done")))
	k.Halt()
	if !k.IsHalted() {
		t.Error("IsHalted should be true after Halt")
	}
	_, _, err := k.Run(context.Background(), "hi")
	if !errors.Is(err, runtime.ErrHalted) {
		t.Errorf("got err=%v, want ErrHalted", err)
	}
	k.Resume()
	if k.IsHalted() {
		t.Error("IsHalted should be false after Resume")
	}
	// After resume, a new mock run works.
	k2 := newKernel(t, mock.New(mock.FinalText("again")))
	if _, _, err := k2.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run after Resume: %v", err)
	}
}

func TestHalt_CancelsInflightRun(t *testing.T) {
	// Provider that blocks until ctx cancellation.
	prov := &blockingProvider{}
	k := newKernel(t, prov)

	done := make(chan error, 1)
	go func() {
		_, _, err := k.Run(context.Background(), "hang")
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	k.Halt()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got err=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight run did not unblock after Halt")
	}
}

// TestRunWith_TimesOut — a run whose wall-clock exceeds Config.MaxDuration
// is cancelled with context.DeadlineExceeded (M31), and the agent loop's
// M30 terminal emitter records task.failed(reason=timeout).
func TestRunWith_TimesOut(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    &blockingProvider{}, // never returns until ctx done
		Tools:       map[string]agent.Tool{"shell": shell.New()},
		MaxDuration: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	start := time.Now()
	_, corr, runErr := k.Run(context.Background(), "hang forever")
	if !errors.Is(runErr, context.DeadlineExceeded) {
		t.Fatalf("got err=%v, want context.DeadlineExceeded", runErr)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("run took %v, want it cut off near the 30ms deadline", elapsed)
	}

	// The M30 terminal event must classify this as a timeout, not a
	// generic error or a cancel.
	var failReason string
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskFailed && e.CorrelationID == corr {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			failReason = p.Reason
		}
		return nil
	})
	if failReason != "timeout" {
		t.Errorf("task.failed reason = %q want timeout", failReason)
	}
}

// TestRunWith_HaltBeatsTimeout — with a per-run timeout armed, an explicit
// Halt before the deadline still cancels with context.Canceled (not
// DeadlineExceeded), so an operator halt stays distinguishable from a
// wall-clock timeout in the failure reason (M30/M31).
func TestRunWith_HaltBeatsTimeout(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    &blockingProvider{},
		Tools:       map[string]agent.Tool{"shell": shell.New()},
		MaxDuration: 10 * time.Second, // far longer than the test
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	done := make(chan error, 1)
	go func() {
		_, _, e := k.Run(context.Background(), "hang")
		done <- e
	}()
	time.Sleep(50 * time.Millisecond)
	k.Halt()

	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Errorf("got err=%v, want context.Canceled (halt, not timeout)", e)
		}
		if errors.Is(e, context.DeadlineExceeded) {
			t.Error("halt was misclassified as a deadline timeout")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("halted run did not unblock")
	}
}

// TestRunWith_CompletesUnderTimeout — a fast run with a generous timeout
// finishes normally; the deadline must not interfere with the happy path.
func TestRunWith_CompletesUnderTimeout(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    mock.New(mock.FinalText("done")),
		Tools:       map[string]agent.Tool{},
		MaxDuration: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ans, _, runErr := k.Run(context.Background(), "hi")
	if runErr != nil {
		t.Fatalf("Run under generous timeout: %v", runErr)
	}
	if ans != "done" {
		t.Errorf("ans = %q want done", ans)
	}
}

// TestCancelRun_CancelsOneRunNotKernel — CancelRun(corr) cancels exactly
// that run (→ context.Canceled, → task.failed reason=canceled) without
// halting the kernel, so other/new runs are unaffected (M32). This is the
// key difference from Halt, which cancels everything and blocks new runs.
func TestCancelRun_CancelsOneRunNotKernel(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: &blockingProvider{},
		Tools:    map[string]agent.Tool{"shell": shell.New()},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	corr := k.NewCorrelation()
	done := make(chan error, 1)
	go func() {
		_, e := k.RunWith(context.Background(), corr, "hang")
		done <- e
	}()
	time.Sleep(50 * time.Millisecond)

	if !k.CancelRun(corr) {
		t.Fatal("CancelRun returned false for a live run")
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Errorf("got err=%v, want context.Canceled", e)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled run did not unblock")
	}

	// The kernel must NOT be halted — CancelRun is targeted, not global.
	if k.IsHalted() {
		t.Error("CancelRun must not halt the kernel")
	}

	// The M30 terminal event must classify this as a cancel, not a timeout
	// or generic error.
	var failReason string
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindTaskFailed && e.CorrelationID == corr {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			failReason = p.Reason
		}
		return nil
	})
	if failReason != "canceled" {
		t.Errorf("task.failed reason = %q want canceled", failReason)
	}

	// A new run is still accepted (proves the kernel wasn't halted): start
	// another blocking run, confirm it's live (cancellable), then clean up.
	corr2 := k.NewCorrelation()
	done2 := make(chan error, 1)
	go func() {
		_, e := k.RunWith(context.Background(), corr2, "hang again")
		done2 <- e
	}()
	time.Sleep(50 * time.Millisecond)
	if !k.CancelRun(corr2) {
		t.Error("second run was not live — kernel may have been wrongly halted")
	}
	<-done2
}

// TestCancelRun_UnknownReturnsFalse — cancelling a correlation with no live
// run is a no-op that reports false (already finished / never existed).
func TestCancelRun_UnknownReturnsFalse(t *testing.T) {
	k := newKernel(t, mock.New(mock.FinalText("ok")))
	if k.CancelRun("run-does-not-exist") {
		t.Error("CancelRun returned true for an unknown correlation")
	}
	// A completed run is no longer live, so cancelling it also reports false.
	_, corr, err := k.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if k.CancelRun(corr) {
		t.Error("CancelRun returned true for an already-finished run")
	}
}

func TestWhy_ReturnsCorrelationGroup(t *testing.T) {
	k := newKernel(t, mock.New(
		mock.ToolUse("c1", "shell", map[string]string{"command": "echo ok"}),
		mock.FinalText("ok printed"),
	))
	ans, _, err := k.Run(context.Background(), "list things")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ans, "ok") {
		t.Errorf("ans=%q", ans)
	}

	// Pick any event from the run and ask Why.
	var someID string
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindToolInvoked {
			someID = e.ID
		}
		return nil
	})
	if someID == "" {
		t.Fatal("test setup: no tool.invoked event found")
	}
	events, err := k.Why(someID)
	if err != nil {
		t.Fatalf("Why: %v", err)
	}
	// Expect: task.received, llm.request, llm.response, tool.invoked, tool.result, llm.request, llm.response, task.completed → 8
	if len(events) < 6 {
		t.Errorf("Why returned %d events; expected the full run", len(events))
	}
	for _, e := range events {
		if e.CorrelationID == "" {
			t.Errorf("event %s missing correlation_id", e.ID)
		}
	}
}

func TestVerify_AfterRun_IsClean(t *testing.T) {
	k := newKernel(t, mock.New(mock.FinalText("done")))
	if _, _, err := k.Run(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if err := k.Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestConcurrentRuns_HaveDistinctCorrelations(t *testing.T) {
	// Construct a provider that returns a final answer immediately. With
	// concurrent runs, every correlation_id must be unique.
	k := newKernel(t, &alwaysFinalProvider{})
	const n = 10
	corrs := make(chan string, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			_, c, err := k.Run(context.Background(), "x")
			if err != nil {
				t.Errorf("Run: %v", err)
				corrs <- ""
				return
			}
			corrs <- c
		})
	}
	wg.Wait()
	close(corrs)
	seen := make(map[string]bool)
	for c := range corrs {
		if c == "" {
			continue
		}
		if seen[c] {
			t.Errorf("duplicate correlation_id %s", c)
		}
		seen[c] = true
	}
}

// ----- helpers -----

type blockingProvider struct{}

func (b *blockingProvider) Name() string { return "blocking" }
func (b *blockingProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type alwaysFinalProvider struct{}

func (a *alwaysFinalProvider) Name() string { return "always-final" }
func (a *alwaysFinalProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: "ok"},
		StopReason: agent.StopEndTurn,
	}, nil
}

func TestKernel_Reload_InvokesOnReloadAndRefreshesCatalog(t *testing.T) {
	called := 0
	var onReloadErr error
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{},
		OnReload: func() error {
			called++
			return onReloadErr
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	cat, providersReloaded, err := k.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if cat == nil {
		t.Error("Reload returned nil catalog")
	}
	if !providersReloaded {
		t.Error("providersReloaded=false but OnReload was configured")
	}
	if called != 1 {
		t.Errorf("OnReload called %d times, want 1", called)
	}

	// Surfaced errors from OnReload should propagate.
	onReloadErr = errors.New("vault locked")
	_, providersReloaded, err = k.Reload()
	if err == nil || !strings.Contains(err.Error(), "vault locked") {
		t.Errorf("Reload should propagate OnReload errors, got %v", err)
	}
	if providersReloaded {
		t.Error("providersReloaded=true after OnReload error")
	}
}

func TestKernel_Reload_NilOnReloadIsCatalogOnly(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{},
		// OnReload deliberately nil — the "no daemon-supplied rebuild"
		// path. Must succeed and report providersReloaded=false.
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	_, providersReloaded, err := k.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if providersReloaded {
		t.Error("providersReloaded=true but OnReload was nil")
	}
}
