// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// probeTool is a side-effect-free tool used to exercise the live-approval path
// without running a real capability (shell/file). It records whether it was
// invoked so a test can assert grant ran it / deny blocked it. Its name maps to
// the edict capability "approvalprobe" (the default toolmap rule), which the
// tests pin to LevelAsk so it requires approval under AskPrompt.
type probeTool struct{ invoked *int32 }

func (probeTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "approvalprobe",
		Description: "test probe (no side effects)",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func (p probeTool) Invoke(_ context.Context, _ json.RawMessage) (agent.Result, error) {
	atomic.AddInt32(p.invoked, 1)
	return agent.Result{Output: "probe ran"}, nil
}

// newApprovalKernel builds a kernel whose probe tool requires live approval:
// the edict engine pins the "approvalprobe" capability to LevelAsk with
// AskPolicy=AskPrompt, so a call routes through the approval.Registry.
func newApprovalKernel(t *testing.T, prov agent.Provider, invoked *int32, timeout time.Duration) (*runtime.Kernel, *approval.Registry) {
	t.Helper()
	eng := edict.New(edict.Options{
		Levels:    map[edict.Capability]edict.TrustLevel{"approvalprobe": edict.LevelAsk},
		AskPolicy: edict.AskPrompt,
	})
	reg := approval.New(approval.Config{Timeout: timeout})
	k, err := runtime.Open(runtime.Config{
		BaseDir:   t.TempDir(),
		Provider:  prov,
		Tools:     map[string]agent.Tool{"approvalprobe": probeTool{invoked: invoked}},
		Edict:     eng,
		Approvals: reg,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k, reg
}

// waitForPending blocks until the registry has exactly one pending request (the
// run goroutine is parked in approval.Submit), or fails the test on timeout.
func waitForPending(t *testing.T, reg *approval.Registry) approval.Request {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		if reg.PendingCount() == 1 {
			return reg.Pending()[0]
		}
		select {
		case <-deadline:
			t.Fatal("no pending approval appeared — the run never routed the tool call to the registry")
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// TestRunWith_ApprovalGrantedRunsTool: an Ask-class tool call under AskPrompt
// pauses the run, surfaces a pending approval with the right tool/capability, and
// — once granted — proceeds to actually invoke the tool. This is the live-HITL
// glue (runtime policyHook → approval.Registry → verdict) end to end.
func TestRunWith_ApprovalGrantedRunsTool(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("c1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, reg := newApprovalKernel(t, prov, &invoked, 5*time.Second)

	type result struct {
		ans string
		err error
	}
	done := make(chan result, 1)
	go func() {
		ans, _, err := k.Run(context.Background(), "go")
		done <- result{ans, err}
	}()

	req := waitForPending(t, reg)
	if req.ToolName != "approvalprobe" || req.Capability != "approvalprobe" {
		t.Errorf("pending request = {tool:%q cap:%q}, want approvalprobe/approvalprobe", req.ToolName, req.Capability)
	}
	if err := reg.Resolve(req.ID, approval.DecisionGrant, "ok by op", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Run: %v", r.err)
		}
		if r.ans != "done" {
			t.Errorf("answer = %q, want done", r.ans)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after the approval was granted")
	}
	if n := atomic.LoadInt32(&invoked); n != 1 {
		t.Errorf("granted tool ran %d times, want 1", n)
	}
}

// TestRunWith_ApprovalDeniedBlocksTool: when the operator denies, the tool must
// NOT execute, and the run still completes (the loop feeds the denial back and
// the model produces its final answer).
func TestRunWith_ApprovalDeniedBlocksTool(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("c1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	k, reg := newApprovalKernel(t, prov, &invoked, 5*time.Second)

	type result struct {
		ans string
		err error
	}
	done := make(chan result, 1)
	go func() {
		ans, _, err := k.Run(context.Background(), "go")
		done <- result{ans, err}
	}()

	req := waitForPending(t, reg)
	if err := reg.Resolve(req.ID, approval.DecisionDeny, "denied by op", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Run: %v", r.err)
		}
		if r.ans != "done" {
			t.Errorf("answer = %q, want done (run completes after denial)", r.ans)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after the approval was denied")
	}
	if n := atomic.LoadInt32(&invoked); n != 0 {
		t.Errorf("denied tool ran %d times, want 0 (must be blocked)", n)
	}
}

// TestRunWith_ApprovalTimeoutBlocksTool: if no operator ever decides, the approval
// times out and the call fails CLOSED — the tool must not execute. This is the
// security-critical default: an unattended Ask-class call is denied, not allowed.
func TestRunWith_ApprovalTimeoutBlocksTool(t *testing.T) {
	var invoked int32
	prov := mock.New(
		mock.ToolUse("c1", "approvalprobe", map[string]any{}),
		mock.FinalText("done"),
	)
	// Short timeout, and we deliberately never Resolve — Submit returns
	// DecisionTimeout, which policyHook maps to deny (fail closed).
	k, _ := newApprovalKernel(t, prov, &invoked, 60*time.Millisecond)

	type result struct {
		ans string
		err error
	}
	done := make(chan result, 1)
	go func() {
		ans, _, err := k.Run(context.Background(), "go")
		done <- result{ans, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Run: %v", r.err)
		}
		if r.ans != "done" {
			t.Errorf("answer = %q, want done (run completes after the approval times out)", r.ans)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish after the approval timed out")
	}
	if n := atomic.LoadInt32(&invoked); n != 0 {
		t.Errorf("timed-out tool ran %d times, want 0 (must fail closed)", n)
	}
}
