// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// Reuses the package's existing blockingProvider (blocks in Complete until the
// run's context is cancelled) and the mock provider for immediate completion.

func waitForTicket(t *testing.T, k *runtime.Kernel, corr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok, _ := k.ResumeStore().Get(corr); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ticket for %s never appeared", corr)
}

// TestResume_TicketDeletedOnCleanCompletion: a run that finishes normally leaves
// no ticket behind, so a clean boot resumes nothing.
func TestResume_TicketDeletedOnCleanCompletion(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(mock.FinalText("done")), ResumeEnabled: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.RunWith(context.Background(), "corr-clean", "do the thing"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if _, ok, _ := k.ResumeStore().Get("corr-clean"); ok {
		t.Fatal("ticket should be deleted after clean completion")
	}
}

// TestResume_TicketKeptWhenInterruptedBySuspend: a run cancelled while the daemon
// is suspending keeps its ticket (→ resumed on restart), and the ticket carries
// the resolved dispatch context needed to re-run it.
func TestResume_TicketKeptWhenInterruptedBySuspend(t *testing.T) {
	prov := &blockingProvider{}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov, ResumeEnabled: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := k.RunWith(ctx, "corr-suspend", "long task")
		done <- e
	}()
	waitForTicket(t, k, "corr-suspend")

	// Simulate a shutdown: classify runs as resumable, THEN cancel them.
	if n := k.Suspend("test"); n != 1 {
		t.Fatalf("Suspend marked %d runs, want 1", n)
	}
	cancel()
	<-done

	tk, ok, err := k.ResumeStore().Get("corr-suspend")
	if err != nil || !ok {
		t.Fatalf("ticket should be KEPT after suspend-interrupt: ok=%v err=%v", ok, err)
	}
	if tk.Status != resume.StatusSuspended {
		t.Fatalf("ticket status = %q, want suspended", tk.Status)
	}
	if tk.Intent != "long task" || tk.Kind != resume.KindRun {
		t.Fatalf("ticket lost dispatch metadata: %+v", tk)
	}
}

// TestResume_TicketDeletedOnOperatorCancel: cancelling a single run while the
// daemon is NOT suspending is an operator kill, not a restart — its ticket is
// deleted so it does not spuriously resume.
func TestResume_TicketDeletedOnOperatorCancel(t *testing.T) {
	prov := &blockingProvider{}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov, ResumeEnabled: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	done := make(chan error, 1)
	go func() {
		_, e := k.RunWith(context.Background(), "corr-cancel", "long task")
		done <- e
	}()
	waitForTicket(t, k, "corr-cancel")

	if !k.CancelRun("corr-cancel") {
		t.Fatal("CancelRun found no live run")
	}
	<-done

	if _, ok, _ := k.ResumeStore().Get("corr-cancel"); ok {
		t.Fatal("ticket should be deleted after operator cancel (not a shutdown)")
	}
}

// TestResume_ReDispatchSeedsConversationAndClears exercises the resumer's exact
// dispatch contract (what buildResumer does): a suspended ticket is re-dispatched
// with WithResumeOwned + WithResumeSeed, the run continues from the saved
// conversation, and the ticket is cleared on clean completion via ResumeFinalize.
func TestResume_ReDispatchSeedsConversationAndClears(t *testing.T) {
	prov := mock.New(mock.FinalText("resumed-done"))
	var seen []agent.Message
	var mu sync.Mutex
	prov.OnRequest = func(req agent.CompletionRequest) {
		mu.Lock()
		if seen == nil {
			seen = append([]agent.Message(nil), req.Messages...)
		}
		mu.Unlock()
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov, ResumeEnabled: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	// A ticket left behind by a prior interrupted run.
	prior := []agent.Message{
		{Role: agent.RoleUser, Content: "orig intent"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "noop", Input: []byte(`{}`)}}},
		{Role: agent.RoleTool, ToolCallID: "c1", Content: "earlier work"},
	}
	if err := k.ResumeStore().Put(&resume.Ticket{
		Corr: "corr-redispatch", Intent: "orig intent", Kind: resume.KindRun,
		Status: resume.StatusSuspended, Resumable: true, Messages: prior, Iter: 2,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	// Re-dispatch exactly as buildResumer does.
	ctx := runtime.WithResumeOwned(context.Background(), resume.KindRun)
	ctx = runtime.WithResumeSeed(ctx, prior, 2)
	ans, err := k.RunWith(ctx, "corr-redispatch", "orig intent")
	k.ResumeFinalize("corr-redispatch", err)
	if err != nil {
		t.Fatalf("resumed RunWith: %v", err)
	}
	if ans != "resumed-done" {
		t.Fatalf("answer = %q, want resumed-done", ans)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 || seen[2].Content != "earlier work" {
		t.Fatalf("resumed run did not continue from the saved conversation: %+v", seen)
	}
	// Ticket cleared on clean completion.
	if _, ok, _ := k.ResumeStore().Get("corr-redispatch"); ok {
		t.Fatal("ticket should be cleared after a resumed run completes")
	}
}
