// SPDX-License-Identifier: MIT

package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/scheduler"
)

// TestContextInvariantMonitor covers the baseline monitor: it returns ctx.Err(),
// so a cancelled context invalidates the plan and a live one passes.
func TestContextInvariantMonitor(t *testing.T) {
	if err := scheduler.ContextInvariantMonitor(context.Background(), scheduler.InvariantSnapshot{}); err != nil {
		t.Fatalf("live context should pass, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := scheduler.ContextInvariantMonitor(ctx, scheduler.InvariantSnapshot{}); err == nil {
		t.Fatalf("cancelled context should invalidate the plan")
	}
}

// TestRun_NilBusPlanCompletion covers the e.bus == nil branch of the plan/node
// lifecycle publishers (publishPlanStarted/Completed, publishNodeStarted/
// Completed): an Executor with no bus runs a plan to completion without
// publishing.
func TestRun_NilBusPlanCompletion(t *testing.T) {
	e := scheduler.New(scheduler.Config{}) // nil bus
	plan := scheduler.Plan{
		Name:  "nobus-ok",
		Nodes: []scheduler.Node{&fakeNode{NodeID: "a", ResultOutput: "done"}},
	}
	res, err := e.Run(context.Background(), plan, "corr-nobus")
	if err != nil {
		t.Fatalf("Run with nil bus: %v", err)
	}
	if res == nil || res.NodeResults["a"].Output != "done" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// TestRun_NilBusPlanFailure covers the e.bus == nil branch of the failure
// publishers (publishPlanFailed, publishNodeFailed): a failing node in a
// bus-less Executor still records the error without publishing.
func TestRun_NilBusPlanFailure(t *testing.T) {
	e := scheduler.New(scheduler.Config{}) // nil bus
	plan := scheduler.Plan{
		Name:  "nobus-fail",
		Nodes: []scheduler.Node{&fakeNode{NodeID: "a", Err: errors.New("boom")}},
	}
	res, err := e.Run(context.Background(), plan, "corr-nobus-fail")
	if err == nil {
		t.Fatalf("Run should surface the node failure")
	}
	if res == nil || len(res.Errors) == 0 {
		t.Fatalf("expected a recorded node error, got %+v", res)
	}
}

// TestLoopNode_Run_DirectNoCorrelation covers LoopNode.Run's IntentFn override
// and the correlationFromCtx fallback (empty string) when Run is invoked with a
// bare context that carries no plan correlation.
func TestLoopNode_Run_DirectNoCorrelation(t *testing.T) {
	var gotIntent, gotCorr string
	n := &scheduler.LoopNode{
		NodeID: "loop1",
		// IntentFn overrides Intent and receives the upstream Inputs.
		IntentFn: func(_ scheduler.Inputs) string { return "derived intent" },
		Runner: func(_ context.Context, intent, corr string) (string, error) {
			gotIntent, gotCorr = intent, corr
			return "answer", nil
		},
	}
	res, err := n.Run(context.Background(), scheduler.Inputs{})
	if err != nil {
		t.Fatalf("LoopNode.Run: %v", err)
	}
	if gotIntent != "derived intent" {
		t.Fatalf("IntentFn override not used, got %q", gotIntent)
	}
	// No correlation in ctx → corr is "<empty>.loop.loop1".
	if gotCorr != ".loop.loop1" {
		t.Fatalf("correlationFromCtx fallback wrong, corr=%q", gotCorr)
	}
	if res.Output != "answer" {
		t.Fatalf("unexpected output %q", res.Output)
	}
}

// TestLoopNode_Run_Errors covers the guard branches of LoopNode.Run: a missing
// Runner, an empty intent, and a Runner that returns an error.
func TestLoopNode_Run_Errors(t *testing.T) {
	// Runner not set.
	if _, err := (&scheduler.LoopNode{NodeID: "x", Intent: "hi"}).Run(context.Background(), scheduler.Inputs{}); err == nil {
		t.Fatalf("expected error when Runner is nil")
	}
	// Empty intent.
	if _, err := (&scheduler.LoopNode{
		NodeID: "x",
		Runner: func(context.Context, string, string) (string, error) { return "", nil },
	}).Run(context.Background(), scheduler.Inputs{}); err == nil {
		t.Fatalf("expected error when intent is empty")
	}
	// Runner returns an error.
	if _, err := (&scheduler.LoopNode{
		NodeID: "x",
		Intent: "go",
		Runner: func(context.Context, string, string) (string, error) {
			return "", errors.New("runner boom")
		},
	}).Run(context.Background(), scheduler.Inputs{}); err == nil {
		t.Fatalf("expected error when Runner fails")
	}
}

// TestGateNode_Run_NoApprovalsRegistry covers the Approvals == nil guard of
// GateNode.Run.
func TestGateNode_Run_NoApprovalsRegistry(t *testing.T) {
	if _, err := (&scheduler.GateNode{NodeID: "g"}).Run(context.Background(), scheduler.Inputs{}); err == nil {
		t.Fatalf("expected error when Approvals registry is nil")
	}
}

// TestGateNode_Run_DefaultCapabilityAndDescription covers the default branches
// for Capability ("plan.gate") and Description when both are left empty on the
// GateNode, resolving the pending approval with a grant.
func TestGateNode_Run_DefaultCapabilityAndDescription(t *testing.T) {
	b, _ := newBus(t)
	apr := approval.New(approval.Config{Bus: b, Timeout: 2 * time.Second})
	gate := &scheduler.GateNode{NodeID: "gate"} // no Capability, no Description
	gate.Approvals = apr

	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if apr.PendingCount() == 1 {
				req := apr.Pending()[0]
				if req.Capability != "plan.gate" {
					t.Errorf("default capability = %q, want plan.gate", req.Capability)
				}
				_ = apr.Resolve(req.ID, approval.DecisionGrant, "ok", "test")
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Errorf("approval never appeared")
	}()

	res, err := gate.Run(context.Background(), scheduler.Inputs{})
	if err != nil {
		t.Fatalf("GateNode.Run granted: %v", err)
	}
	if res.Detail["decision"] != "grant" {
		t.Fatalf("expected grant decision, got %+v", res.Detail)
	}
}

// TestRun_InvariantRejectsAtPlanStart covers the checkInvariant(InvariantPlanStart)
// failure path: a monitor that rejects at plan start aborts before any node runs
// (publishPlanFailed + return), and pickReady sees the plan already invalidated.
func TestRun_InvariantRejectsAtPlanStart(t *testing.T) {
	b, _ := newBus(t)
	monErr := errors.New("plan-start veto")
	e := scheduler.New(scheduler.Config{
		Bus: b,
		Monitor: func(_ context.Context, s scheduler.InvariantSnapshot) error {
			if s.Phase == scheduler.InvariantPlanStart {
				return monErr
			}
			return nil
		},
	})
	plan := scheduler.Plan{
		Name:  "vetoed",
		Nodes: []scheduler.Node{&fakeNode{NodeID: "a", ResultOutput: "x"}},
	}
	_, err := e.Run(context.Background(), plan, "corr-veto")
	if !errors.Is(err, scheduler.ErrPlanInvalidated) {
		t.Fatalf("Run err=%v, want ErrPlanInvalidated", err)
	}
}

// TestRun_PickReadySeesInvalidated covers the `invalidated` early return in
// pickReady. A slow independent node keeps inflight > 0 while the monitor rejects
// a sibling node; when pickReady is next called, it sees invalidated=true and
// returns nil.
func TestRun_PickReadySeesInvalidated(t *testing.T) {
	b, _ := newBus(t)
	errReject := errors.New("reject")
	e := scheduler.New(scheduler.Config{
		Bus: b,
		Monitor: func(_ context.Context, s scheduler.InvariantSnapshot) error {
			if s.Phase == scheduler.InvariantNodeStart && s.NodeID == "childA" {
				return errReject
			}
			return nil
		},
	})
	slow := &fakeNode{NodeID: "slow", Sleep: 200 * time.Millisecond, ResultOutput: "ok"}
	root := &fakeNode{NodeID: "root", ResultOutput: "r"}
	childA := &fakeNode{NodeID: "childA", Deps: []string{"root"}, ResultOutput: "a"}
	childB := &fakeNode{NodeID: "childB", Deps: []string{"root"}, ResultOutput: "b"}
	plan := scheduler.Plan{
		Name:  "invalidated",
		Nodes: []scheduler.Node{slow, root, childA, childB},
	}
	_, err := e.Run(context.Background(), plan, "")
	if err == nil {
		t.Fatal("expected plan failure after childA was rejected")
	}
}

// TestRun_ErrorKeysWithRunNodeFailure covers errorKeys' for-range body by
// having node "a" fail at Run time (adding its error to errs), then node "c"
// become ready after "b" completes and pass through checkInvariant with the
// populated errs map.
func TestRun_ErrorKeysWithRunNodeFailure(t *testing.T) {
	b, _ := newBus(t)
	// Must have a non-nil Monitor so checkInvariant calls invariantSnapshot
	// (and thus errorKeys). ContextInvariantMonitor works: it accepts all phases
	// when the context is live.
	e := scheduler.New(scheduler.Config{
		Bus:     b,
		Monitor: scheduler.ContextInvariantMonitor,
	})
	// "a" fails at Run time. "c" depends on "b" (independent of "a").
	// When "c" becomes ready and reaches checkInvariant, errs already contains
	// "a"'s error which exercises errorKeys' loop body.
	// b runs slowly so the driver loop processes c's invariant check
	// in a separate iteration, reaching errorKeys with a populated errs map.
	plan := scheduler.Plan{
		Name: "errkeys-runfail",
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "a", Err: errors.New("a fails")},
			&fakeNode{NodeID: "b", Sleep: 50 * time.Millisecond, ResultOutput: "ok"},
			&fakeNode{NodeID: "c", Deps: []string{"b"}, ResultOutput: "done"},
		},
	}
	res, err := e.Run(context.Background(), plan, "")
	if err == nil {
		t.Fatal("expected plan error since 'a' failed")
	}
	if res.NodeResults["c"].Output != "done" {
		t.Errorf("c should have completed: %+v", res.NodeResults)
	}
	if _, ok := res.Errors["a"]; !ok {
		t.Errorf("a's error should be recorded")
	}
}

// TestRun_MonitorFailsNodeWithDependents covers failNodeWithoutRun's dependency-
// decrement loop (a monitor rejects an upstream node that has a downstream
// dependent) and the invalidated→nil early return in pickReady on the next pass.
func TestRun_MonitorFailsNodeWithDependents(t *testing.T) {
	b, _ := newBus(t)
	monErr := errors.New("node-start veto on up")
	e := scheduler.New(scheduler.Config{
		Bus: b,
		Monitor: func(_ context.Context, s scheduler.InvariantSnapshot) error {
			if s.Phase == scheduler.InvariantNodeStart && s.NodeID == "up" {
				return monErr
			}
			return nil
		},
	})
	plan := scheduler.Plan{
		Name: "veto-upstream",
		Nodes: []scheduler.Node{
			&fakeNode{NodeID: "up", ResultOutput: "x"},
			&fakeNode{NodeID: "down", Deps: []string{"up"}, ResultOutput: "never"},
		},
	}
	_, err := e.Run(context.Background(), plan, "corr-veto-up")
	if !errors.Is(err, scheduler.ErrPlanInvalidated) {
		t.Fatalf("Run err=%v, want ErrPlanInvalidated", err)
	}
}
