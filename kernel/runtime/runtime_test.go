// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/kernel/event"
	"github.com/ersinkoc/agezt/kernel/runtime"
	"github.com/ersinkoc/agezt/plugins/providers/mock"
	"github.com/ersinkoc/agezt/plugins/tools/shell"
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
